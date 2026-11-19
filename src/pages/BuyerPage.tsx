/**
 * Buyer page: browse products, choose quantities and submit an order request.
 * The product browsing and cart features are implemented in a later phase.
 */
export function BuyerPage() {
  return (
    <>
      <header className="page-header">
        <h1 className="page-header__title">Place an order request</h1>
        <p className="page-header__description">
          Browse the catalogue, choose quantities and submit a request for the sales team to
          review.
        </p>
      </header>
      <section className="card">
        <p className="placeholder-note">
          The product catalogue and request form are added in a later phase.
        </p>
      </section>
    </>
  );
}
