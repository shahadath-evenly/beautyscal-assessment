import { Spinner } from "./Spinner";

export interface LoadingStateProps {
  message?: string;
  /** Renders at the height of a table body, so a listing does not jump when
   *  the rows arrive. */
  variant?: "block" | "inline";
}

/** The shared loading placeholder. */
export function LoadingState({ message = "Loading…", variant = "block" }: LoadingStateProps) {
  if (variant === "inline") {
    return (
      <span className="loading-state loading-state--inline">
        <Spinner size="sm" label={message} />
        <span className="loading-state__message">{message}</span>
      </span>
    );
  }

  return (
    <div className="loading-state">
      <Spinner label={message} />
      <p className="loading-state__message">{message}</p>
    </div>
  );
}
