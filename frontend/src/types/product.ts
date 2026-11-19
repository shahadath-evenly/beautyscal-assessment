/** A product in the current workspace's catalogue. */
export interface Product {
  id: string;
  name: string;
  /** Decimal string such as "49.90". Never a number: see money handling. */
  price: string;
  currency: string;
  stock: number;
  createdAt: string;
  updatedAt: string;
}
