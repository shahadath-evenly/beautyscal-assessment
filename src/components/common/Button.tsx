import type { ButtonHTMLAttributes, ReactNode } from "react";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger" | "success";
export type ButtonSize = "sm" | "md";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Shows a spinner and disables the button while an action is in flight. */
  loading?: boolean;
  children: ReactNode;
}

/**
 * The application's button.
 *
 * `loading` disables the button as well as showing the spinner, which is what
 * stops a double click from submitting a request or accepting an order twice.
 */
export function Button({
  variant = "secondary",
  size = "md",
  loading = false,
  disabled,
  className,
  children,
  type = "button",
  ...rest
}: ButtonProps) {
  const classes = [
    "button",
    `button--${variant}`,
    `button--${size}`,
    loading ? "button--loading" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button
      type={type}
      className={classes}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {loading && <span className="button__spinner" aria-hidden="true" />}
      <span>{children}</span>
    </button>
  );
}
