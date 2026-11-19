export interface SpinnerProps {
  /** Accessible description of what is loading. */
  label?: string;
  size?: "sm" | "md" | "lg";
}

/** An indeterminate activity indicator. */
export function Spinner({ label = "Loading", size = "md" }: SpinnerProps) {
  return (
    <span className={`spinner spinner--${size}`} role="status" aria-live="polite">
      <span className="spinner__circle" aria-hidden="true" />
      <span className="visually-hidden">{label}</span>
    </span>
  );
}
