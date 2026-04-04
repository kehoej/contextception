/** Application entry point. */

import { getServerPort, APP_NAME } from "./config";
import { requestLogger } from "./middleware/logging";
import { requireAuth } from "./middleware/auth";
import * as routes from "./routes";

export function createApp() {
  const port = getServerPort();
  console.log(`Starting ${APP_NAME} on port ${port}`);

  return {
    port,
    routes: {
      "GET /users/:id": requireAuth(routes.getUser),
      "POST /users": routes.createUser,
      "POST /login": routes.login,
    },
    middleware: [requestLogger],
  };
}
