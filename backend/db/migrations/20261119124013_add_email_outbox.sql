-- Transactional outbox for buyer notifications.
--
-- An acceptance and the intent to notify the buyer are written in the same
-- transaction, so the two can never disagree: if the order exists, the email
-- event exists, and if the acceptance rolls back the event goes with it. The
-- send itself happens later, in a worker, so a slow or failing email provider
-- can never delay or roll back an accepted order.
--
-- The rendered subject and body are stored on the row rather than rebuilt at
-- send time. The buyer is told what was agreed when the request was accepted,
-- and a later price change must not rewrite a message already owed to them.

CREATE TABLE email_outbox (
    id                  UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    workspace_id        UUID        NOT NULL,
    order_request_id    UUID        NOT NULL,
    event_type          TEXT        NOT NULL,

    recipient_email     TEXT        NOT NULL,
    recipient_name      TEXT        NOT NULL,
    subject             TEXT        NOT NULL,
    body_text           TEXT        NOT NULL,
    body_html           TEXT        NOT NULL,

    -- pending: due to be sent. sending: claimed by a worker. sent: accepted
    -- by the provider. failed: attempts exhausted, left for inspection.
    status              TEXT        NOT NULL DEFAULT 'pending',
    attempts            INTEGER     NOT NULL DEFAULT 0,
    max_attempts        INTEGER     NOT NULL DEFAULT 5,
    next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error          TEXT,
    provider_message_id TEXT,
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT email_outbox_workspace_id_fkey
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE,

    -- Tenant-aware composite reference, like every other link between
    -- tenant-owned tables: an event cannot point at another workspace's
    -- request. ON DELETE CASCADE because an event is meaningless without the
    -- request it describes.
    CONSTRAINT email_outbox_order_request_fkey
        FOREIGN KEY (workspace_id, order_request_id)
        REFERENCES order_requests (workspace_id, id) ON DELETE CASCADE,

    -- One event of a kind per request. The buyer is told once that their
    -- request was accepted, however many times the insert is attempted.
    CONSTRAINT email_outbox_request_event_key
        UNIQUE (workspace_id, order_request_id, event_type),

    CONSTRAINT email_outbox_event_type_valid
        CHECK (event_type IN ('order_request_accepted')),
    CONSTRAINT email_outbox_status_valid
        CHECK (status IN ('pending', 'sending', 'sent', 'failed')),
    CONSTRAINT email_outbox_attempts_non_negative CHECK (attempts >= 0),
    CONSTRAINT email_outbox_max_attempts_positive CHECK (max_attempts > 0),
    CONSTRAINT email_outbox_recipient_email_format
        CHECK (recipient_email ~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'),
    CONSTRAINT email_outbox_subject_not_blank CHECK (length(btrim(subject)) > 0),
    -- A sent event must record when, and nothing else may claim to have been.
    CONSTRAINT email_outbox_sent_at_consistent
        CHECK ((status = 'sent') = (sent_at IS NOT NULL))
);

-- The worker's only hot query: the oldest due events. Partial, because rows
-- that are sent or exhausted are never polled again and would otherwise grow
-- the index without bound.
CREATE INDEX email_outbox_due_idx
    ON email_outbox (next_attempt_at, created_at)
    WHERE status IN ('pending', 'sending');

-- Supports inspecting a workspace's notification history.
CREATE INDEX email_outbox_workspace_id_created_at_idx
    ON email_outbox (workspace_id, created_at DESC);
