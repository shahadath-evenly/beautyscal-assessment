import { useState } from "react";

import { PageHeader } from "../components/common";
import { DEFAULT_PAGE_SIZE } from "../service";
import { useDebouncedValue } from "../hooks/useDebouncedValue";
import { useProducts } from "../features/products/useProducts";
import { ProductCatalogue } from "../features/buyer/ProductCatalogue";
import { RequestConfirmation } from "../features/buyer/RequestConfirmation";
import { RequestSummary, type BuyerDetails } from "../features/buyer/RequestSummary";
import { useCart } from "../features/buyer/useCart";
import { useCreateOrderRequest } from "../features/buyer/useCreateOrderRequest";
import { validateRequest, type BuyerFieldErrors } from "../features/buyer/validation";
import type { OrderRequestDetail } from "../types/orderRequest";

/**
 * The buyer workflow: browse the catalogue, choose quantities, give contact
 * details and submit a request.
 */
export function BuyerPage() {
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(DEFAULT_PAGE_SIZE);
  const [search, setSearch] = useState("");
  // Typing issues one request when the buyer pauses, not one per keystroke.
  const debouncedSearch = useDebouncedValue(search, 300);

  const [buyer, setBuyer] = useState<BuyerDetails>({ name: "", email: "" });
  const [fieldErrors, setFieldErrors] = useState<BuyerFieldErrors>({});
  const [confirmed, setConfirmed] = useState<OrderRequestDetail | null>(null);

  const cart = useCart();
  const products = useProducts({ page, limit, search: debouncedSearch });
  const submission = useCreateOrderRequest();

  const changeSearch = (value: string) => {
    setSearch(value);
    // A new search invalidates the current page number: page 3 of the old
    // results is meaningless for the new ones.
    setPage(1);
  };

  const changeLimit = (value: number) => {
    setLimit(value);
    setPage(1);
  };

  const submit = () => {
    const errors = validateRequest(buyer, cart);
    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;

    submission.mutate(
      {
        buyerName: buyer.name.trim(),
        buyerEmail: buyer.email.trim(),
        // Only products and quantities are sent. Prices come from the
        // server's catalogue; it would reject them here.
        items: cart.lines.map((line) => ({
          productId: line.product.id,
          quantity: line.quantity,
        })),
      },
      {
        onSuccess: (created) => {
          setConfirmed(created);
          cart.clear();
          setFieldErrors({});
        },
      },
    );
  };

  const startAnother = () => {
    setConfirmed(null);
    setBuyer({ name: "", email: "" });
    submission.reset();
    // Stock has not changed yet — acceptance does that — but the catalogue is
    // refetched so a long-lived page does not show a stale one.
    void products.refetch();
  };

  if (confirmed) {
    return (
      <>
        <PageHeader
          title="Request submitted"
          description="Your request is now with the brand's sales team."
        />
        <RequestConfirmation request={confirmed} onStartAnother={startAnother} />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Place an order request"
        description="Browse the catalogue, choose quantities and submit a request for the sales team to review."
      />

      <div className="buyer-layout">
        <ProductCatalogue
          page={page}
          limit={limit}
          search={search}
          onSearchChange={changeSearch}
          onPageChange={setPage}
          onLimitChange={changeLimit}
          data={products.data}
          loading={products.isPending}
          error={products.error}
          onRetry={() => void products.refetch()}
          cart={cart}
        />

        <RequestSummary
          cart={cart}
          buyer={buyer}
          onBuyerChange={setBuyer}
          fieldErrors={fieldErrors}
          onSubmit={submit}
          submitting={submission.isPending}
          submitError={submission.error}
        />
      </div>
    </>
  );
}
