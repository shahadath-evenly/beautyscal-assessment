import { Combobox, SearchInput } from "../../components/common";
import type { OrderRequestStatus } from "../../types/orderRequest";

/** "" is All; the others are the statuses the API accepts. */
export type StatusFilter = OrderRequestStatus | "";

const STATUS_OPTIONS: Array<{ value: StatusFilter; label: string }> = [
  { value: "", label: "All statuses" },
  { value: "new", label: "New" },
  { value: "accepted", label: "Accepted" },
  { value: "rejected", label: "Rejected" },
];

export interface RequestFiltersProps {
  search: string;
  onSearchChange: (value: string) => void;
  status: StatusFilter;
  onStatusChange: (value: StatusFilter) => void;
}

/**
 * Search and status filter.
 *
 * Both are applied by the API — the search runs as a parameterised SQL filter
 * over buyer name and email — so this component only reports what was chosen.
 */
export function RequestFilters({
  search,
  onSearchChange,
  status,
  onStatusChange,
}: RequestFiltersProps) {
  return (
    <div className="filters">
      <SearchInput
        id="request-search"
        label="Search requests by buyer name or email"
        placeholder="Search buyer name or email…"
        value={search}
        onChange={onSearchChange}
      />

      <div className="filters__field">
        <label className="filters__label" htmlFor="request-status">
          Status
        </label>
        <Combobox
          id="request-status"
          value={status}
          onChange={(next) => onStatusChange(next as StatusFilter)}
          options={STATUS_OPTIONS.map((option) => ({
            value: option.value,
            label: option.label,
          }))}
          placeholder="All statuses"
          emptyMessage="No matching status"
        />
      </div>
    </div>
  );
}
