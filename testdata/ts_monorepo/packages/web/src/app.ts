/** Main web application. */

import React from "react";
import { logger } from "@myapp/shared";
import { createServer } from "@myapp/api";
import { HomePage } from "./pages/home";
import { UserPage } from "./pages/users";

export function App() {
  logger.info("Initializing web app");
  const server = createServer(3000);

  return {
    pages: [HomePage, UserPage],
    server,
  };
}
