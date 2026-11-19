import { Alert, Button, Card, EmptyState, Field, Input } from "../../components/common";
import { describeError } from "../../components/common/errorMessages";
import { formatMoney } from "../../utils/format";
import type { Cart } from "./useCart";

export interface BuyerDetails {
  name: string;
  email: string;
}

export interface RequestSummaryProps {
  cart: Cart;
  buyer: BuyerDetails;
  onBuyerChange: (buyer: BuyerDetails) => void;
  fieldErrors: Partial<Record<"name" | "email" | "items", string>>;
  onSubmit: () => void;
  submitting: boolean;
  submitError: unknown;
}

/**
 * The cart, the buyer's details and the submit control.
 *
 * Amounts shown here are computed exactly from the prices the API returned,
 * but they are only a preview: the server recalculates every line from its
 * own catalogue, so a stale price is corrected rather than honoured.
 */
export function RequestSummary({
  cart,
  buyer,
  onBuyerChange,
  fieldErrors,
  onSubmit,
  submitting,
  submitError,
}: RequestSummaryProps) {
  return (
    <Card title="Your request" description="Review the lines, then submit for review.">
      {cart.isEmpty ? (
        <EmptyState
          title="No products selected"
          description="Pick quantities in the catalogue to build your request."
        />
      ) : (
        <>
          <ul className="cart">
            {cart.lines.map((line) => (
              <li className="cart__line" key={line.product.id}>
                <div className="cart__line-main">
                  <span className="row-primary">{line.product.name}</span>
                  <span className="row-secondary">
                    {line.quantity} × {formatMoney(line.product.price, line.product.currency)}
                  </span>
                </div>

                <span className="row-money cart__line-total">
                  {formatMoney(line.lineTotal, line.product.currency)}
                </span>

                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => cart.remove(line.product.id)}
                  aria-label={`Remove ${line.product.name} from the request`}
                >
                  Remove
                </Button>
              </li>
            ))}
          </ul>

          <div className="cart__total">
            <span>Total</span>
            <strong className="row-money">
              {formatMoney(cart.total, cart.currency ?? "")}
            </strong>
          </div>
        </>
      )}

      <form
        className="buyer-form"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
        noValidate
      >
        <Field htmlFor="buyer-name" label="Your name" required error={fieldErrors.name}>
          <Input
            id="buyer-name"
            value={buyer.name}
            autoComplete="name"
            invalid={Boolean(fieldErrors.name)}
            onChange={(event) => onBuyerChange({ ...buyer, name: event.target.value })}
          />
        </Field>

        <Field
          htmlFor="buyer-email"
          label="Email"
          required
          error={fieldErrors.email}
          hint="The sales team replies to this address."
        >
          <Input
            id="buyer-email"
            type="email"
            value={buyer.email}
            autoComplete="email"
            invalid={Boolean(fieldErrors.email)}
            onChange={(event) => onBuyerChange({ ...buyer, email: event.target.value })}
          />
        </Field>

        {fieldErrors.items && (
          <Alert tone="warning">{fieldErrors.items}</Alert>
        )}

        {submitError != null && <SubmitError error={submitError} />}

        <Button
          type="submit"
          variant="primary"
          loading={submitting}
          // Disabled while in flight, so a second click cannot submit a
          // duplicate request.
          disabled={cart.isEmpty || submitting}
        >
          {submitting ? "Submitting…" : "Submit request"}
        </Button>
      </form>
    </Card>
  );
}

function SubmitError({ error }: { error: unknown }) {
  return (
    <div className="buyer-form__error">
      <Alert tone="danger" title="Could not submit the request">
        <ErrorDetail error={error} />
      </Alert>
    </div>
  );
}

/** Renders the server's message and, when it sent them, the field problems. */
function ErrorDetail({ error }: { error: unknown }) {
  const described = describeError(error);

  return (
    <>
      <p className="alert__message">{described.title}</p>
      {described.description && (
        <p className="alert__message alert__message--muted">{described.description}</p>
      )}
    </>
  );
}
