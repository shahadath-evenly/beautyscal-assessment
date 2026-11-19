import { useEffect, useId, useRef, type ReactNode } from "react";

export interface ModalProps {
  open: boolean;
  title: string;
  description?: string;
  onClose: () => void;
  /** Rendered in the footer; the caller owns the actions. */
  footer?: ReactNode;
  children: ReactNode;
  /** Blocks dismissal while an action is in flight. */
  dismissible?: boolean;
  size?: "sm" | "lg";
}

/**
 * A modal dialog.
 *
 * Escape and the backdrop close it, the panel takes focus when it opens and
 * focus returns to whatever opened it on close, so keyboard users are not
 * dropped at the top of the page. While an action is in flight the caller can
 * set `dismissible={false}`, which stops a stray Escape from hiding the
 * result of a mutation that is still running.
 */
export function Modal({
  open,
  title,
  description,
  onClose,
  footer,
  children,
  dismissible = true,
  size = "lg",
}: ModalProps) {
  const titleId = useId();
  const descriptionId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const openerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;

    openerRef.current = document.activeElement as HTMLElement | null;
    panelRef.current?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && dismissible) onClose();
    };
    document.addEventListener("keydown", onKeyDown);

    // The page behind must not scroll while the dialog is up.
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
      openerRef.current?.focus?.();
    };
  }, [open, dismissible, onClose]);

  if (!open) return null;

  return (
    <div
      className="modal"
      onMouseDown={(event) => {
        // Only a click that both starts and ends on the backdrop closes it,
        // so a drag that ends outside the panel does not dismiss the dialog.
        if (event.target === event.currentTarget && dismissible) onClose();
      }}
    >
      <div
        className={`modal__panel modal__panel--${size}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descriptionId : undefined}
        ref={panelRef}
        tabIndex={-1}
      >
        <header className="modal__header">
          <div>
            <h2 className="modal__title" id={titleId}>
              {title}
            </h2>
            {description && (
              <p className="modal__description" id={descriptionId}>
                {description}
              </p>
            )}
          </div>
          <button
            type="button"
            className="modal__close"
            onClick={onClose}
            disabled={!dismissible}
            aria-label="Close dialog"
          >
            ×
          </button>
        </header>

        <div className="modal__body">{children}</div>

        {footer && <footer className="modal__footer">{footer}</footer>}
      </div>
    </div>
  );
}
