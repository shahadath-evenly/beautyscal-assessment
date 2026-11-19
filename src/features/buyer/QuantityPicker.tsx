import { Button, Input } from "../../components/common";
import type { Product } from "../../types/product";

export interface QuantityPickerProps {
  product: Product;
  quantity: number;
  onChange: (quantity: number) => void;
  /** When set, the control is disabled and the reason is shown. */
  blockedReason: string | null;
}

/**
 * Chooses how many of a product to request.
 *
 * The maximum is the stock the server reported, so the control cannot express
 * an impossible quantity. The server checks stock again when the request is
 * accepted, which is the check that actually matters.
 */
export function QuantityPicker({ product, quantity, onChange, blockedReason }: QuantityPickerProps) {
  if (blockedReason) {
    return <span className="row-muted quantity-picker__blocked">{blockedReason}</span>;
  }

  const atMax = quantity >= product.stock;
  const inputId = `quantity-${product.id}`;

  return (
    <div className="quantity-picker">
      <Button
        size="sm"
        variant="secondary"
        onClick={() => onChange(quantity - 1)}
        disabled={quantity <= 0}
        aria-label={`Remove one ${product.name}`}
      >
        −
      </Button>

      <Input
        id={inputId}
        className="quantity-picker__input"
        type="number"
        inputMode="numeric"
        min={0}
        max={product.stock}
        value={quantity === 0 ? "" : String(quantity)}
        placeholder="0"
        aria-label={`Quantity of ${product.name}, up to ${product.stock} available`}
        onChange={(event) => {
          const parsed = Number.parseInt(event.target.value, 10);
          onChange(Number.isNaN(parsed) ? 0 : parsed);
        }}
      />

      <Button
        size="sm"
        variant="secondary"
        onClick={() => onChange(quantity + 1)}
        disabled={atMax}
        aria-label={
          atMax
            ? `No more ${product.name} available`
            : `Add one ${product.name}`
        }
      >
        +
      </Button>
    </div>
  );
}
