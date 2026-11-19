import { Card, PageHeader } from "../components/common";

/**
 * Buyer page: browse products, choose quantities and submit a request.
 * The catalogue and cart arrive in a later phase.
 */
export function BuyerPage() {
  return (
    <>
      <PageHeader
        title="Place an order request"
        description="Browse the catalogue, choose quantities and submit a request for the sales team to review."
      />
      <Card>
        <p className="placeholder-note">
          The product catalogue and request form are added in the next phase.
        </p>
      </Card>
    </>
  );
}
