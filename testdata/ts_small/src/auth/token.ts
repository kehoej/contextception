/** Token generation and validation. */

import type { AuthToken } from "../types";
import { generateId } from "../utils";

const TOKEN_EXPIRY_MS = 3600 * 1000; // 1 hour

export function createToken(userId: string): AuthToken {
  return {
    token: `${userId}.${generateId()}`,
    expiresAt: Date.now() + TOKEN_EXPIRY_MS,
  };
}

export function isTokenValid(token: AuthToken): boolean {
  return token.expiresAt > Date.now();
}
