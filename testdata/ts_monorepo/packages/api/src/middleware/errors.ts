/** Error handling middleware. */

import { AppError } from "@myapp/shared/errors";
import { logger } from "@myapp/shared";

export function errorHandler(err: unknown, req: any, res: any) {
  if (err instanceof AppError) {
    logger.warn(`${err.name}: ${err.message}`);
    return res.status(err.statusCode).json({ error: err.message });
  }

  logger.error(`Unexpected error: ${err}`);
  return res.status(500).json({ error: "Internal server error" });
}
