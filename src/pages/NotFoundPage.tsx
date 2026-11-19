import { Link } from "react-router-dom";

import { paths } from "../routes/paths";

/** Shown for any unknown route. */
export function NotFoundPage() {
  return (
    <>
      <header className="page-header">
        <h1 className="page-header__title">Page not found</h1>
        <p className="page-header__description">
          That page does not exist. It may have moved.
        </p>
      </header>
      <section className="card">
        <Link to={paths.buyer}>Back to the buyer page</Link>
      </section>
    </>
  );
}
