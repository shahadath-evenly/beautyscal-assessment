import type { InputHTMLAttributes } from "react";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

/** A text input carrying the application's styling and invalid state. */
export function Input({ invalid, className, id, ...rest }: InputProps) {
  const classes = ["input", invalid ? "input--invalid" : "", className ?? ""]
    .filter(Boolean)
    .join(" ");

  return (
    <input
      id={id}
      className={classes}
      aria-invalid={invalid || undefined}
      aria-describedby={invalid && id ? `${id}-error` : undefined}
      {...rest}
    />
  );
}
