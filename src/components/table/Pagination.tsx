import { Button, Select } from "../common";
import { PAGE_SIZE_OPTIONS } from "../../service";
import type { PaginationState } from "./types";

/**
 * Pagination controls for a server-paginated listing.
 *
 * Every number shown comes from the API's `meta`, so the control never
 * guesses how many rows exist or paginates in the browser.
 */
export function Pagination({ page, limit, total, onPageChange, onLimitChange }: PaginationState) {
  const totalPages = total > 0 ? Math.ceil(total / limit) : 1;
  const firstRow = total === 0 ? 0 : (page - 1) * limit + 1;
  const lastRow = Math.min(page * limit, total);

  return (
    <div className="pagination">
      <p className="pagination__summary" aria-live="polite">
        {total === 0
          ? "No results"
          : `Showing ${firstRow}–${lastRow} of ${total}`}
      </p>

      <div className="pagination__controls">
        {onLimitChange && (
          <label className="pagination__size">
            <span className="pagination__size-label">Per page</span>
            <Select
              value={String(limit)}
              onChange={(event) => onLimitChange(Number(event.target.value))}
              options={PAGE_SIZE_OPTIONS.map((size) => ({
                value: String(size),
                label: String(size),
              }))}
              aria-label="Results per page"
            />
          </label>
        )}

        <div className="pagination__pages">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onPageChange(page - 1)}
            disabled={page <= 1}
          >
            Previous
          </Button>
          <span className="pagination__position">
            Page {page} of {totalPages}
          </span>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onPageChange(page + 1)}
            disabled={page >= totalPages}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
