/** Home page. */

import { logger } from "@myapp/shared";

export function HomePage() {
  logger.info("Rendering home page");
  return "<h1>Welcome</h1>";
}
