import { Input } from "./Input";

export interface SearchInputProps {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  /** Accessible name, since the control carries no visible label. */
  label: string;
}

/**
 * A search box. Debouncing belongs to the caller, which owns the query, so
 * this component stays controlled and predictable.
 */
export function SearchInput({
  id = "search",
  value,
  onChange,
  placeholder = "Search…",
  label,
}: SearchInputProps) {
  return (
    <div className="search-input">
      <Input
        id={id}
        type="search"
        value={value}
        aria-label={label}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      {value && (
        <button
          type="button"
          className="search-input__clear"
          onClick={() => onChange("")}
          aria-label="Clear search"
        >
          ×
        </button>
      )}
    </div>
  );
}
