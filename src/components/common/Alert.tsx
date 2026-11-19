import type { ReactNode } from "react";

export type AlertTone = "info" | "success" | "warning" | "danger";

export interface AlertProps {
  tone?: AlertTone;
  title?: string;
  children: ReactNode;
  onDismiss?: () => void;
}

/**
 * An inline message. Success and informational messages are polite live
 * regions; warnings and failures assert, so they are announced promptly.
 */
export function Alert({ tone = "info", title, children, onDismiss }: AlertProps) {
  const assertive = tone === "danger" || tone === "warning";

  return (
    <div
      className={`alert alert--${tone}`}
      role={assertive ? "alert" : "status"}
      aria-live={assertive ? "assertive" : "polite"}
    >
      <div className="alert__body">
        {title && <p className="alert__title">{title}</p>}
        <div className="alert__message">{children}</div>
      </div>
      {onDismiss && (
        <button
          type="button"
          className="alert__dismiss"
          onClick={onDismiss}
          aria-label="Dismiss message"
        >
          ×
        </button>
      )}
    </div>
  );
}
