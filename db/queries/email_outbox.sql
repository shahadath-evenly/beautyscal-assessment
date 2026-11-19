-- Outbox queries. The enqueue runs inside the acceptance transaction; the
-- rest belong to the background worker.

-- EnqueueEmail records the intent to notify a buyer. ON CONFLICT DO NOTHING
-- against the one-event-per-request constraint: a retried acceptance must not
-- queue a second message, and a missing row here must never fail an
-- acceptance that has already reserved stock.
--
-- name: EnqueueEmail :one
INSERT INTO email_outbox (
    workspace_id, order_request_id, event_type,
    recipient_email, recipient_name,
    subject, body_text, body_html, max_attempts
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (workspace_id, order_request_id, event_type) DO NOTHING
RETURNING *;

-- ClaimDueEmails takes the next batch of due events for this worker.
--
-- FOR UPDATE SKIP LOCKED is what makes several workers safe: each skips rows
-- another has locked rather than queueing behind them, so no event is claimed
-- twice and none blocks. The claim moves the row to 'sending' and counts the
-- attempt in the same statement, so a crash mid-send cannot produce an
-- infinite retry loop.
--
-- name: ClaimDueEmails :many
UPDATE email_outbox
SET status     = 'sending',
    attempts   = attempts + 1,
    updated_at = now()
WHERE id IN (
    SELECT id
    FROM email_outbox
    WHERE status = 'pending'
      AND next_attempt_at <= now()
    ORDER BY next_attempt_at, created_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- ReleaseStaleClaims returns events to the queue after a worker died holding
-- them. Without it a crash between claim and send would strand the row in
-- 'sending' forever. The attempt is already counted, so a repeatedly crashing
-- send still exhausts its budget.
--
-- name: ReleaseStaleClaims :many
UPDATE email_outbox
SET status          = 'pending',
    next_attempt_at = now(),
    last_error      = 'reclaimed after a worker stopped mid-send',
    updated_at      = now()
WHERE status = 'sending'
  AND updated_at < now() - sqlc.arg(stale_after)::interval
RETURNING id;

-- MarkEmailSent closes an event the provider accepted.
--
-- name: MarkEmailSent :exec
UPDATE email_outbox
SET status              = 'sent',
    sent_at             = now(),
    provider_message_id = sqlc.narg(provider_message_id),
    last_error          = NULL,
    updated_at          = now()
WHERE id = sqlc.arg(id);

-- RescheduleEmail puts a failed attempt back in the queue after a delay. The
-- caller computes the backoff, so the policy lives in one readable place in
-- Go rather than in SQL.
--
-- name: RescheduleEmail :exec
UPDATE email_outbox
SET status          = 'pending',
    next_attempt_at = now() + sqlc.arg(retry_in)::interval,
    last_error      = sqlc.arg(last_error),
    updated_at      = now()
WHERE id = sqlc.arg(id);

-- MarkEmailFailed gives up on an event: either the attempt budget is spent or
-- the provider rejected it in a way retrying cannot fix. The row is kept, with
-- its error, so the failure is visible rather than silent.
--
-- name: MarkEmailFailed :exec
UPDATE email_outbox
SET status     = 'failed',
    last_error = sqlc.arg(last_error),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- GetEmailEvent reads one event, for tests and for inspecting a delivery.
--
-- name: GetEmailEvent :one
SELECT * FROM email_outbox WHERE id = $1;

-- ListEmailEventsForRequest shows what was queued for a request.
--
-- name: ListEmailEventsForRequest :many
SELECT *
FROM email_outbox
WHERE workspace_id = $1
  AND order_request_id = $2
ORDER BY created_at;

-- CountEmailsByStatus backs a simple operational view of the queue.
--
-- name: CountEmailsByStatus :many
SELECT status, count(*) AS total
FROM email_outbox
GROUP BY status
ORDER BY status;
