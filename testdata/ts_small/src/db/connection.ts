/** Database connection management. */

import pg from "pg";
import { getDbConfig } from "../config";
import type { DbConfig } from "../types";

let pool: any = null;

export function getConnection(): any {
  if (!pool) {
    const config = getDbConfig();
    pool = createPool(config);
  }
  return pool;
}

function createPool(config: DbConfig): any {
  return { config, connected: true };
}

export function closeConnection(): void {
  if (pool) {
    pool = null;
  }
}
