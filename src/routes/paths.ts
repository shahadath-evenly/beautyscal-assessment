/** Route paths, kept in one place so links and route definitions agree. */
export const paths = {
  buyer: "/",
  salesRequests: "/sales/requests",
  salesRequestDetail: "/sales/requests/:requestId",
} as const;

/** Builds the sales detail path for a specific request. */
export function salesRequestDetailPath(requestId: string): string {
  return `/sales/requests/${requestId}`;
}
