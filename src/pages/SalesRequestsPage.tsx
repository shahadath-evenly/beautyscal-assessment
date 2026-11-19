/**
 * Sales page: list order requests, filter by status and open one to accept or
 * reject it. The listing and actions are implemented in a later phase.
 */
export function SalesRequestsPage() {
  return (
    <>
      <header className="page-header">
        <h1 className="page-header__title">Order requests</h1>
        <p className="page-header__description">
          Review incoming buyer requests, then accept to create a draft order or reject the
          request.
        </p>
      </header>
      <section className="card">
        <p className="placeholder-note">
          The request listing, filters and accept/reject actions are added in a later phase.
        </p>
      </section>
    </>
  );
}
