import { useState } from "react";

import {
  Alert,
  Button,
  Card,
  ConfirmDialog,
  PageHeader,
  describeError,
} from "../components/common";
import { OrderRequestRow } from "../components/rows";
import { DataTable } from "../components/table";
import type { Column } from "../components/table";
import { DEFAULT_PAGE_SIZE } from "../service";
import { useDebouncedValue } from "../hooks/useDebouncedValue";
import { RequestDetail } from "../features/order-requests/RequestDetail";
import { RequestFilters, type StatusFilter } from "../features/order-requests/RequestFilters";
import { useOrderRequests } from "../features/order-requests/useOrderRequests";
import { useRequestActions } from "../features/order-requests/useRequestActions";
import type { OrderRequest } from "../types/orderRequest";

/** The listing's columns. The row renderer owns the cells; these supply the
 *  headers, widths and the mobile behaviour. */
const COLUMNS: Column<OrderRequest>[] = [
  { id: "buyer", header: "Buyer" },
  { id: "status", header: "Status", width: "120px" },
  { id: "items", header: "Lines", align: "right", width: "90px", hideOnMobile: true },
  { id: "total", header: "Total", align: "right", width: "140px" },
  { id: "created", header: "Submitted", width: "180px", hideOnMobile: true },
];

interface Feedback {
  tone: "success" | "danger";
  title: string;
  message?: string;
}

/**
 * The sales workflow: review incoming requests, then accept one to create a
 * draft order and reserve stock, or reject it.
 */
export function SalesRequestsPage() {
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(DEFAULT_PAGE_SIZE);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>("");
  // One request when typing pauses, rather than one per keystroke.
  const debouncedSearch = useDebouncedValue(search, 300);

  const [openId, setOpenId] = useState<string | null>(null);
  const [confirmRejectId, setConfirmRejectId] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<Feedback | null>(null);

  const requests = useOrderRequests({
    page,
    limit,
    search: debouncedSearch,
    status,
  });
  const { accept, reject, pendingId, busy } = useRequestActions();

  const rows = requests.data?.data ?? [];
  const meta = requests.data?.meta;

  // A new search or page size invalidates the page number: page 3 of the
  // previous results is meaningless for the new ones.
  const changeSearch = (value: string) => {
    setSearch(value);
    setPage(1);
  };
  const changeStatus = (value: StatusFilter) => {
    setStatus(value);
    setPage(1);
  };
  const changeLimit = (value: number) => {
    setLimit(value);
    setPage(1);
  };

  const runAccept = (requestId: string) => {
    // A second click while the first is in flight would be a second attempt
    // to reserve stock; the backend answers it with 409, but there is no
    // reason to send it.
    if (busy) return;
    setFeedback(null);

    accept.mutate(requestId, {
      onSuccess: (result) => {
        setFeedback({
          tone: "success",
          title: `Request from ${result.request.buyerName} accepted`,
          message: `Draft order ${result.order.id} was created and stock reserved.`,
        });
      },
      onError: (error) => {
        const described = describeError(error);
        setFeedback({
          tone: "danger",
          title: described.title,
          message: described.description ?? "No stock was reserved.",
        });
      },
    });
  };

  const runReject = (requestId: string) => {
    if (busy) return;
    setFeedback(null);

    reject.mutate(requestId, {
      onSuccess: (rejected) => {
        setConfirmRejectId(null);
        setFeedback({
          tone: "success",
          title: `Request from ${rejected.buyerName} rejected`,
          message: "No order was created and stock is unchanged.",
        });
      },
      onError: (error) => {
        setConfirmRejectId(null);
        const described = describeError(error);
        setFeedback({
          tone: "danger",
          title: described.title,
          message: described.description,
        });
      },
    });
  };

  /** The failure from the last action on this request, if it concerned it. */
  const actionErrorFor = (requestId: string): unknown => {
    if (accept.isError && accept.variables === requestId) return accept.error;
    if (reject.isError && reject.variables === requestId) return reject.error;
    return undefined;
  };

  const emptyTitle = debouncedSearch
    ? `No requests match “${debouncedSearch}”`
    : status
      ? `No ${status} requests`
      : "No order requests yet";

  return (
    <>
      <PageHeader
        title="Order requests"
        description="Review incoming buyer requests, then accept to create a draft order or reject the request."
      />

      {feedback && (
        <Alert
          tone={feedback.tone}
          title={feedback.title}
          onDismiss={() => setFeedback(null)}
        >
          {feedback.message}
        </Alert>
      )}

      <Card flush>
        <div className="listing__toolbar">
          <RequestFilters
            search={search}
            onSearchChange={changeSearch}
            status={status}
            onStatusChange={changeStatus}
          />
        </div>

        <DataTable<OrderRequest>
          columns={COLUMNS}
          data={rows}
          getRowId={(row) => row.id}
          renderRow={({ row, actions }) => (
            <OrderRequestRow request={row} actions={actions} />
          )}
          // The table knows nothing about these: View, Accept and Reject are
          // this page's, and a decided request offers only View.
          rowActions={(row) => (
            <div className="row-actions">
              <Button size="sm" variant="secondary" onClick={() => setOpenId(row.id)}>
                View
              </Button>
              {row.status === "new" && (
                <>
                  <Button
                    size="sm"
                    variant="success"
                    onClick={() => runAccept(row.id)}
                    loading={accept.isPending && accept.variables === row.id}
                    disabled={busy}
                  >
                    Accept
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    onClick={() => setConfirmRejectId(row.id)}
                    disabled={busy}
                  >
                    Reject
                  </Button>
                </>
              )}
            </div>
          )}
          loading={requests.isPending}
          error={requests.error}
          onRetry={() => void requests.refetch()}
          emptyTitle={emptyTitle}
          emptyMessage={
            debouncedSearch || status
              ? "Try a different search or status filter."
              : "Requests submitted on the buyer page appear here."
          }
          caption="Order requests in the current workspace"
          pagination={{
            page: meta?.page ?? page,
            limit: meta?.limit ?? limit,
            total: meta?.total ?? 0,
            onPageChange: setPage,
            onLimitChange: changeLimit,
          }}
        />
      </Card>

      <RequestDetail
        requestId={openId}
        onClose={() => setOpenId(null)}
        onAccept={runAccept}
        onReject={(requestId) => setConfirmRejectId(requestId)}
        pending={pendingId !== null && pendingId === openId}
        actionError={openId ? actionErrorFor(openId) : undefined}
      />

      <ConfirmDialog
        open={confirmRejectId !== null}
        title="Reject this request?"
        message="The buyer's request will be marked rejected. No order is created and stock is not touched. This cannot be undone."
        confirmLabel="Reject request"
        tone="danger"
        pending={reject.isPending}
        onConfirm={() => confirmRejectId && runReject(confirmRejectId)}
        onCancel={() => setConfirmRejectId(null)}
      />
    </>
  );
}
