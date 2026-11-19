import { Card, PageHeader } from "../components/common";

/**
 * Sales page: review requests, then accept or reject them.
 * The listing and actions arrive in a later phase.
 */
export function SalesRequestsPage() {
  return (
    <>
      <PageHeader
        title="Order requests"
        description="Review incoming buyer requests, then accept to create a draft order or reject the request."
      />
      <Card>
        <p className="placeholder-note">
          The request listing, filters and accept/reject actions are added in a later phase.
        </p>
      </Card>
    </>
  );
}
