import { Button } from "./Button";
import { describeError } from "./errorMessages";

export interface ErrorStateProps {
  error: unknown;
  /** Shown as a retry control when provided. */
  onRetry?: () => void;
  /** Replaces the title derived from the error. */
  title?: string;
}

/** The shared error panel. */
export function ErrorState({ error, onRetry, title }: ErrorStateProps) {
  const described = describeError(error);

  return (
    <div className="error-state" role="alert">
      <p className="error-state__title">{title ?? described.title}</p>
      {described.description && (
        <p className="error-state__description">{described.description}</p>
      )}
      {onRetry && (
        <div className="error-state__action">
          <Button variant="secondary" size="sm" onClick={onRetry}>
            Try again
          </Button>
        </div>
      )}
    </div>
  );
}
