import type { ReactNode } from "react";

import { EmptyState, ErrorState, LoadingState } from "../common";
import { Pagination } from "./Pagination";
import type { Column, PaginationState, RowRendererProps } from "./types";

export interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  /** Stable key for a row; required rather than falling back to the index,
   *  which would reorder badly across pages. */
  getRowId: (row: T) => string;

  /** Optional renderer owning the whole <tr>. Row components live in
   *  components/rows and are passed in by the calling feature. */
  renderRow?: (props: RowRendererProps<T>) => ReactNode;
  /** Controls for one row, rendered in a trailing actions column. The table
   *  knows nothing about what they do. */
  rowActions?: (row: T) => ReactNode;
  /** Header for the actions column. */
  actionsHeader?: ReactNode;

  loading?: boolean;
  error?: unknown;
  onRetry?: () => void;

  emptyTitle?: string;
  emptyMessage?: string;
  emptyAction?: ReactNode;

  /** Server-side pagination. Omit for an unpaginated table. */
  pagination?: PaginationState;

  /** Accessible description of the table's contents. */
  caption?: string;
}

/**
 * The application's one table.
 *
 * It renders columns, rows and pagination, and nothing else: no feature
 * actions are baked in, so a listing shows View, Accept and Reject only
 * because its caller passed them. Pagination is driven entirely by the
 * server's `meta`; the table never slices the data it is given.
 */
export function DataTable<T>({
  columns,
  data,
  getRowId,
  renderRow,
  rowActions,
  actionsHeader = "Actions",
  loading = false,
  error,
  onRetry,
  emptyTitle = "Nothing to show",
  emptyMessage,
  emptyAction,
  pagination,
  caption,
}: DataTableProps<T>) {
  const columnCount = columns.length + (rowActions ? 1 : 0);

  // A failure replaces the rows: showing a stale page beside an error implies
  // the data is current.
  if (error) {
    return (
      <div className="data-table">
        <ErrorState error={error} onRetry={onRetry} />
      </div>
    );
  }

  return (
    <div className="data-table">
      <div className="data-table__scroll">
        <table className="table">
          {caption && <caption className="visually-hidden">{caption}</caption>}

          <thead>
            <tr>
              {columns.map((column) => (
                <th
                  key={column.id}
                  scope="col"
                  style={column.width ? { width: column.width } : undefined}
                  className={[
                    column.align ? `table__cell--${column.align}` : "",
                    column.hideOnMobile ? "table__cell--desktop" : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                >
                  {column.header}
                </th>
              ))}
              {rowActions && (
                <th scope="col" className="table__cell--right">
                  {actionsHeader}
                </th>
              )}
            </tr>
          </thead>

          <tbody>
            {loading && (
              <tr>
                <td colSpan={columnCount} className="table__state">
                  <LoadingState />
                </td>
              </tr>
            )}

            {!loading && data.length === 0 && (
              <tr>
                <td colSpan={columnCount} className="table__state">
                  <EmptyState
                    title={emptyTitle}
                    description={emptyMessage}
                    action={emptyAction}
                  />
                </td>
              </tr>
            )}

            {!loading &&
              data.map((row) => {
                const actions = rowActions ? (
                  <td className="table__cell--right table__cell--actions">{rowActions(row)}</td>
                ) : undefined;

                if (renderRow) {
                  return (
                    <RowWrapper key={getRowId(row)}>
                      {renderRow({ row, columns, actions })}
                    </RowWrapper>
                  );
                }

                return (
                  <tr key={getRowId(row)}>
                    {columns.map((column) => (
                      <td
                        key={column.id}
                        className={[
                          column.align ? `table__cell--${column.align}` : "",
                          column.hideOnMobile ? "table__cell--desktop" : "",
                        ]
                          .filter(Boolean)
                          .join(" ")}
                      >
                        {column.cell ? column.cell(row) : null}
                      </td>
                    ))}
                    {actions}
                  </tr>
                );
              })}
          </tbody>
        </table>
      </div>

      {pagination && <Pagination {...pagination} />}
    </div>
  );
}

/** Keeps the key on a stable element when a custom renderer supplies the row. */
function RowWrapper({ children }: { children: ReactNode }) {
  return <>{children}</>;
}
