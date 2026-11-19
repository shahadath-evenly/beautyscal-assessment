/** The statuses a request can hold. */
export type OrderRequestStatus = "new" | "accepted" | "rejected";

export const ORDER_REQUEST_STATUSES: OrderRequestStatus[] = [
  "new",
  "accepted",
  "rejected",
];

/** One line of a request, with the prices captured when it was submitted. */
export interface OrderRequestItem {
  productId: string;
  productName: string;
  quantity: number;
  unitPrice: string;
  lineTotal: string;
  currency: string;
}

/** A request as it appears in a listing. */
export interface OrderRequest {
  id: string;
  buyerName: string;
  buyerEmail: string;
  status: OrderRequestStatus;
  itemCount: number;
  total: string;
  currency: string;
  createdAt: string;
  updatedAt: string;
}

/** A request with its lines. */
export interface OrderRequestDetail extends OrderRequest {
  items: OrderRequestItem[];
}

/** The draft order created by accepting a request. */
export interface Order {
  id: string;
  status: string;
  createdAt: string;
}

/** The response returned by accepting a request. */
export interface AcceptResult {
  request: OrderRequestDetail;
  order: Order;
}

/** The payload for submitting a request. Prices are deliberately absent: the
 *  server reads them from the catalogue and would reject them here. */
export interface CreateOrderRequestInput {
  buyerName: string;
  buyerEmail: string;
  items: Array<{ productId: string; quantity: number }>;
}
