import type { ReactNode } from "react";

/** One column of a {@link DataTable}. */
export interface Column<T> {
  /** Stable identifier, also used as the React key. */
  id: string;
  /** Header text. */
  header: ReactNode;
  /** Cell contents for a row. Omitted when a custom row renderer is used. */
  cell?: (row: T) => ReactNode;
  align?: "left" | "right" | "center";
  /** A CSS width such as "120px" or "20%". */
  width?: string;
  /** Hides the column below the mobile breakpoint. */
  hideOnMobile?: boolean;
}

/** Server-driven pagination state. */
export interface PaginationState {
  page: number;
  limit: number;
  total: number;
  onPageChange: (page: number) => void;
  onLimitChange?: (limit: number) => void;
}

/** The props a custom row renderer receives. */
export interface RowRendererProps<T> {
  row: T;
  columns: Column<T>[];
  /** Rendered in the trailing actions cell, when the table was given one. */
  actions?: ReactNode;
}
