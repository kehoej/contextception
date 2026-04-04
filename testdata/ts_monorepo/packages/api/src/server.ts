/** Server setup and configuration. */

import { logger } from "@myapp/shared";
import { errorHandler } from "./middleware/errors";
import { userRoutes } from "./routes/users";
import { productRoutes } from "./routes/products";

export function createServer(port: number) {
  logger.info(`Starting API server on port ${port}`);

  return {
    port,
    routes: [userRoutes, productRoutes],
    middleware: [errorHandler],
  };
}
