import { Link } from "react-router-dom";

import { Card, PageHeader } from "../components/common";
import { paths } from "../routes/paths";

/** Shown for any unknown route. */
export function NotFoundPage() {
  return (
    <>
      <PageHeader title="Page not found" description="That page does not exist." />
      <Card>
        <Link to={paths.buyer}>Back to the buyer page</Link>
      </Card>
    </>
  );
}
