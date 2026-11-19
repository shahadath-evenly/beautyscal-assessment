import type { ReactNode } from "react";

export interface FieldProps {
  /** Ties the label to the control; must match the control's id. */
  htmlFor: string;
  label: string;
  /** Inline validation message. Its presence marks the field invalid. */
  error?: string;
  hint?: string;
  required?: boolean;
  children: ReactNode;
}

/**
 * Label, control and message for one form field.
 *
 * The error is rendered in a live region and given an id the control
 * references through `aria-describedby`, so a screen reader announces it.
 */
export function Field({ htmlFor, label, error, hint, required, children }: FieldProps) {
  const describedBy = error ? `${htmlFor}-error` : hint ? `${htmlFor}-hint` : undefined;

  return (
    <div className={`field${error ? " field--invalid" : ""}`}>
      <label className="field__label" htmlFor={htmlFor}>
        {label}
        {required && (
          <span className="field__required" aria-hidden="true">
            *
          </span>
        )}
      </label>

      {children}

      {hint && !error && (
        <p className="field__hint" id={`${htmlFor}-hint`}>
          {hint}
        </p>
      )}
      {error && (
        <p className="field__error" id={`${htmlFor}-error`} role="alert">
          {error}
        </p>
      )}
      {/* Exposed so the control can reference it without each caller wiring
          aria-describedby by hand. */}
      <span hidden data-described-by={describedBy} />
    </div>
  );
}
