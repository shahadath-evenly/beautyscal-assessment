import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";

export interface ComboboxOption {
  value: string;
  label: string;
  /** Secondary text shown under the label. It is searched too. */
  hint?: string;
}

export interface ComboboxProps {
  options: ComboboxOption[];
  value: string;
  onChange: (value: string) => void;
  /** Shown when nothing is selected. */
  placeholder?: string;
  /** Accessible name, for when there is no visible <label>. */
  "aria-label"?: string;
  id?: string;
  disabled?: boolean;
  invalid?: boolean;
  /** Adds a button that resets the value to "". */
  clearable?: boolean;
  /** Shown when the query matches nothing. */
  emptyMessage?: string;
  size?: "sm" | "md";
  /** "top" opens the panel upward, for a control near the bottom edge. */
  placement?: "bottom" | "top";
  className?: string;
}

/** Case-insensitive index of needle in haystack, or -1. */
function indexOfMatch(haystack: string, needle: string): number {
  if (!needle) return -1;
  return haystack.toLowerCase().indexOf(needle.toLowerCase());
}

/**
 * Renders a label with the matched part of the query marked, so it is obvious
 * why a row survived the filter.
 */
function Highlighted({ text, query }: { text: string; query: string }) {
  const at = indexOfMatch(text, query);
  if (at < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, at)}
      <mark className="combobox__mark">{text.slice(at, at + query.length)}</mark>
      {text.slice(at + query.length)}
    </>
  );
}

/**
 * A searchable select.
 *
 * It replaces the native dropdown everywhere in the app, so every list
 * behaves the same: type to filter, and the rest of the best match is
 * completed inline and left selected, so Enter or Tab accepts it and carrying
 * on typing overwrites it. That is the same "autofill" a browser does on an
 * address field, which is why it needs no learning.
 *
 * It follows the ARIA combobox pattern: the input owns the listbox through
 * aria-controls, announces the highlighted row with aria-activedescendant,
 * and every row is a real option with aria-selected. Arrow keys, Home, End,
 * Enter, Escape and Tab all behave as a native select does, so nothing is
 * lost by replacing it.
 */
export function Combobox({
  options,
  value,
  onChange,
  placeholder = "Select…",
  "aria-label": ariaLabel,
  id,
  disabled = false,
  invalid = false,
  clearable = false,
  emptyMessage = "No matches",
  size = "md",
  placement = "bottom",
  className,
}: ComboboxProps) {
  const reactId = useId();
  const inputId = id ?? `combobox-${reactId}`;
  const listId = `${inputId}-list`;

  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);

  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  /**
   * Inline completion must not fight the user. Deleting means they are
   * removing characters, so completing would put them straight back.
   */
  const completingRef = useRef(true);
  /** Selection to apply after the completed text has rendered. */
  const pendingSelectionRef = useRef<[number, number] | null>(null);

  const selected = options.find((option) => option.value === value) ?? null;

  // Prefix matches first: typing "ac" should offer "Accepted" before a row
  // that merely contains those letters.
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    const matches = options.filter(
      (option) =>
        option.label.toLowerCase().includes(q) || (option.hint ?? "").toLowerCase().includes(q),
    );
    return matches.sort((a, b) => {
      const aPrefix = a.label.toLowerCase().startsWith(q) ? 0 : 1;
      const bPrefix = b.label.toLowerCase().startsWith(q) ? 0 : 1;
      return aPrefix - bPrefix;
    });
  }, [options, query]);

  // Apply the caret selection that inline completion asked for, after render
  // rather than during the event, so it lands on the completed text.
  useLayoutEffect(() => {
    const pending = pendingSelectionRef.current;
    if (!pending) return;
    pendingSelectionRef.current = null;
    inputRef.current?.setSelectionRange(pending[0], pending[1]);
  });

  // Keep the highlighted row in view when arrowing through a long list.
  useEffect(() => {
    if (!open) return;
    const node = listRef.current?.children.item(active);
    node?.scrollIntoView({ block: "nearest" });
  }, [active, open]);

  // A click anywhere else closes the list, as a native dropdown does.
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) close();
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  function openList() {
    if (disabled) return;
    setQuery("");
    const at = selected ? options.findIndex((option) => option.value === selected.value) : 0;
    setActive(at < 0 ? 0 : at);
    setOpen(true);
  }

  function close() {
    setOpen(false);
    setQuery("");
  }

  function commit(option: ComboboxOption) {
    onChange(option.value);
    close();
    inputRef.current?.focus();
  }

  function handleChange(event: React.ChangeEvent<HTMLInputElement>) {
    const typed = event.target.value;
    setOpen(true);
    setActive(0);

    const match = typed
      ? options.find((option) => option.label.toLowerCase().startsWith(typed.toLowerCase()))
      : undefined;

    // Complete the rest of the match and leave it selected, so the next
    // keystroke replaces it and Enter accepts it.
    if (completingRef.current && match && match.label.length > typed.length) {
      setQuery(match.label);
      pendingSelectionRef.current = [typed.length, match.label.length];
      return;
    }
    setQuery(typed);
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    // Backspace and Delete are the user undoing a completion.
    completingRef.current = event.key !== "Backspace" && event.key !== "Delete";

    const last = filtered.length - 1;

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        if (!open) {
          openList();
          return;
        }
        setActive((current) => (current >= last ? 0 : current + 1));
        return;
      case "ArrowUp":
        event.preventDefault();
        if (!open) {
          openList();
          return;
        }
        setActive((current) => (current <= 0 ? last : current - 1));
        return;
      case "Home":
        if (!open) return;
        event.preventDefault();
        setActive(0);
        return;
      case "End":
        if (!open) return;
        event.preventDefault();
        setActive(last < 0 ? 0 : last);
        return;
      case "Enter": {
        if (!open) return;
        const option = filtered[active];
        if (option) {
          // Not a form submit: this is the selection being made.
          event.preventDefault();
          commit(option);
        }
        return;
      }
      case "Escape":
        if (!open) return;
        event.preventDefault();
        close();
        return;
      case "Tab":
        // Moving on accepts what is shown, which is what the completed text
        // led the user to expect.
        if (open) {
          const option = filtered[active];
          if (option && query.trim() !== "") commit(option);
          else close();
        }
        return;
      default:
    }
  }

  const classes = [
    "combobox",
    size === "sm" ? "combobox--sm" : "",
    placement === "top" ? "combobox--top" : "",
    open ? "combobox--open" : "",
    invalid ? "combobox--invalid" : "",
    disabled ? "combobox--disabled" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");

  const activeOption = open ? filtered[active] : undefined;

  return (
    <div className={classes} ref={rootRef}>
      <div className="combobox__control">
        <input
          ref={inputRef}
          id={inputId}
          className="combobox__input"
          type="text"
          role="combobox"
          aria-expanded={open}
          aria-controls={listId}
          aria-autocomplete="both"
          aria-activedescendant={activeOption ? `${listId}-${activeOption.value}` : undefined}
          aria-label={ariaLabel}
          aria-invalid={invalid || undefined}
          // The browser's own dropdown would sit on top of this one.
          autoComplete="off"
          spellCheck={false}
          disabled={disabled}
          value={open ? query : (selected?.label ?? "")}
          placeholder={open ? (selected?.label ?? placeholder) : placeholder}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onMouseDown={() => {
            if (!open) openList();
          }}
          onFocus={(event) => event.currentTarget.select()}
          onBlur={() => {
            if (open) close();
          }}
        />

        {clearable && value !== "" && !disabled && (
          <button
            type="button"
            className="combobox__clear"
            aria-label="Clear selection"
            // Fires before blur, so the list does not close first.
            onMouseDown={(event) => {
              event.preventDefault();
              onChange("");
              close();
              inputRef.current?.focus();
            }}
          >
            <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
              <path
                d="M4 4l8 8M12 4l-8 8"
                stroke="currentColor"
                strokeWidth="1.6"
                strokeLinecap="round"
              />
            </svg>
          </button>
        )}

        <button
          type="button"
          className="combobox__toggle"
          // The input already carries the accessible name and the expanded
          // state; this is decorative for anyone using the keyboard.
          tabIndex={-1}
          aria-hidden="true"
          disabled={disabled}
          onMouseDown={(event) => {
            event.preventDefault();
            if (open) close();
            else {
              openList();
              inputRef.current?.focus();
            }
          }}
        >
          <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
            <path
              d="M4 6.5L8 10.5L12 6.5"
              stroke="currentColor"
              strokeWidth="1.6"
              strokeLinecap="round"
              strokeLinejoin="round"
              fill="none"
            />
          </svg>
        </button>
      </div>

      {open && (
        <ul className="combobox__list" id={listId} role="listbox" ref={listRef}>
          {filtered.length === 0 && (
            <li className="combobox__empty" role="presentation">
              {emptyMessage}
            </li>
          )}

          {filtered.map((option, index) => {
            const isSelected = option.value === value;
            return (
              <li
                key={option.value}
                id={`${listId}-${option.value}`}
                role="option"
                aria-selected={isSelected}
                className={[
                  "combobox__option",
                  index === active ? "combobox__option--active" : "",
                  isSelected ? "combobox__option--selected" : "",
                ]
                  .filter(Boolean)
                  .join(" ")}
                // Pointer rather than hover state, so the keyboard and the
                // mouse cannot disagree about which row is highlighted.
                onPointerEnter={() => setActive(index)}
                onMouseDown={(event) => {
                  event.preventDefault();
                  commit(option);
                }}
              >
                <span className="combobox__option-text">
                  <span className="combobox__option-label">
                    <Highlighted text={option.label} query={query.trim()} />
                  </span>
                  {option.hint && (
                    <span className="combobox__option-hint">
                      <Highlighted text={option.hint} query={query.trim()} />
                    </span>
                  )}
                </span>

                {isSelected && (
                  <svg
                    className="combobox__check"
                    viewBox="0 0 16 16"
                    aria-hidden="true"
                    focusable="false"
                  >
                    <path
                      d="M3.5 8.5l3 3l6-6"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      fill="none"
                    />
                  </svg>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
