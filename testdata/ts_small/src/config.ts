/** Application configuration. */

import type { DbConfig } from "./types";

export const APP_NAME = "ts-small-app";
export const APP_VERSION = "1.0.0";

export function getDbConfig(): DbConfig {
  return {
    host: process.env.DB_HOST || "localhost",
    port: parseInt(process.env.DB_PORT || "5432"),
    database: process.env.DB_NAME || "app",
  };
}

export function getServerPort(): number {
  return parseInt(process.env.PORT || "3000");
}
