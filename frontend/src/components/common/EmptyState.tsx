import type { ReactNode } from "react";

export interface EmptyStateProps {
  title: string;
  /** Says what to do next, not merely that there is nothing here. */
  description?: string;
  action?: ReactNode;
}

/** Shown when a listing has no rows. */
export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="empty-state">
      <p className="empty-state__title">{title}</p>
      {description && <p className="empty-state__description">{description}</p>}
      {action && <div className="empty-state__action">{action}</div>}
    </div>
  );
}
