import { Card, SearchInput } from "../../components/common";
import { DataTable, type Column } from "../../components/table";
import { ProductRow } from "../../components/rows";
import type { Product } from "../../types/product";
import type { Paginated } from "../../types/api";
import { QuantityPicker } from "./QuantityPicker";
import type { Cart } from "./useCart";

export interface ProductCatalogueProps {
  page: number;
  limit: number;
  search: string;
  onSearchChange: (search: string) => void;
  onPageChange: (page: number) => void;
  onLimitChange: (limit: number) => void;

  data?: Paginated<Product>;
  loading: boolean;
  error: unknown;
  onRetry: () => void;

  cart: Cart;
}

/** Columns describe the catalogue; the row renderer decides how it looks. */
const columns: Column<Product>[] = [
  { id: "name", header: "Product" },
  { id: "price", header: "Price", align: "right", width: "140px" },
  { id: "stock", header: "Stock", align: "right", width: "120px" },
];

/**
 * The buyer's product catalogue: searchable, paginated by the server, with a
 * quantity control per row supplied as a row action.
 */
export function ProductCatalogue({
  page,
  limit,
  search,
  onSearchChange,
  onPageChange,
  onLimitChange,
  data,
  loading,
  error,
  onRetry,
  cart,
}: ProductCatalogueProps) {
  const products = data?.data ?? [];
  const meta = data?.meta;

  return (
    <Card
      title="Catalogue"
      description="Choose quantities to add them to your request."
      actions={
        <SearchInput
          id="product-search"
          label="Search products by name"
          placeholder="Search products…"
          value={search}
          onChange={onSearchChange}
        />
      }
      flush
    >
      <DataTable<Product>
        caption="Products available in this workspace"
        columns={columns}
        data={products}
        getRowId={(product) => product.id}
        loading={loading}
        error={error}
        onRetry={onRetry}
        emptyTitle={search ? "No matching products" : "No products yet"}
        emptyMessage={
          search
            ? `Nothing matches “${search}”. Try a different search.`
            : "This workspace has no products. Run the seed command to add the demo catalogue."
        }
        renderRow={({ row, actions }) => (
          <ProductRow key={row.id} product={row} actions={actions} />
        )}
        rowActions={(product) => (
          <QuantityPicker
            product={product}
            quantity={cart.quantityOf(product.id)}
            blockedReason={cart.blockedReason(product)}
            onChange={(quantity) => cart.setQuantity(product, quantity)}
          />
        )}
        actionsHeader="Quantity"
        pagination={{
          page,
          limit,
          total: meta?.total ?? 0,
          onPageChange,
          onLimitChange,
        }}
      />
    </Card>
  );
}
