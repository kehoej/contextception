/** Product API routes. */

import { formatResponse, generateId } from "@myapp/shared";
import type { Product } from "@myapp/shared";
import { NotFoundError } from "@myapp/shared/errors";

const products: Product[] = [];

export function productRoutes(req: any, res: any) {
  if (req.method === "GET") {
    const product = products.find((p) => p.id === req.params.id);
    if (!product) throw new NotFoundError("Product");
    return formatResponse(product);
  }

  if (req.method === "POST") {
    const product: Product = {
      id: generateId(),
      name: req.body.name,
      price: req.body.price,
    };
    products.push(product);
    return formatResponse(product, 201);
  }
}
