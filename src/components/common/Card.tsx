import type { ReactNode } from "react";

export interface CardProps {
  title?: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  /** Removes the body padding, for a card whose content is a table. */
  flush?: boolean;
}

/** A white surface with an optional header. */
export function Card({ title, description, actions, children, flush }: CardProps) {
  return (
    <section className="card">
      {(title || actions) && (
        <header className="card__header">
          <div>
            {title && <h2 className="card__title">{title}</h2>}
            {description && <p className="card__description">{description}</p>}
          </div>
          {actions && <div className="card__actions">{actions}</div>}
        </header>
      )}
      <div className={flush ? "card__body card__body--flush" : "card__body"}>{children}</div>
    </section>
  );
}
