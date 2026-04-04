/** Authentication logic. */

import type { User, AuthToken } from "../types";
import { findUserByEmail } from "../db/queries";
import { createToken } from "./token";

export function authenticate(email: string, password: string): AuthToken | null {
  const user = findUserByEmail(email);
  if (!user) return null;
  // In real app, verify password hash
  return createToken(user.id);
}

export function validateCredentials(email: string, password: string): boolean {
  return email.includes("@") && password.length >= 8;
}
