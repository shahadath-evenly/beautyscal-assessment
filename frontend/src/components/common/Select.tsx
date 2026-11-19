import type { SelectHTMLAttributes } from "react";

export interface SelectOption {
  value: string;
  label: string;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  options: SelectOption[];
  /** Shown as the first entry, for an unset value. */
  placeholder?: string;
  invalid?: boolean;
}

/** A native select, styled to match the other controls. */
export function Select({ options, placeholder, invalid, className, ...rest }: SelectProps) {
  const classes = ["select", invalid ? "select--invalid" : "", className ?? ""]
    .filter(Boolean)
    .join(" ");

  return (
    <select className={classes} aria-invalid={invalid || undefined} {...rest}>
      {placeholder && <option value="">{placeholder}</option>}
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}
