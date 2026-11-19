import { useEffect, useState } from "react";

/**
 * Returns `value` after it has stopped changing for `delay` milliseconds.
 *
 * Used for search inputs so typing issues one request when the user pauses,
 * rather than one per keystroke.
 */
export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);

  return debounced;
}
