/** Product list component. */

import type { Product } from "@myapp/shared";
import { formatResponse } from "@myapp/shared";

export function ProductList(products: Product[]) {
  return products.map((p) => `<li>${p.name}: $${p.price}</li>`).join("");
}
