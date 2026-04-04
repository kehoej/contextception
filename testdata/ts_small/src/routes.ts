/** API route definitions. */

import { z } from "zod";
import type { User, ApiResponse } from "./types";
import { formatResponse, sanitizeInput } from "./utils";
import { findUserById, insertUser } from "./db/queries";
import { authenticate } from "./auth/login";
import { requireAuth, requireRole } from "./middleware/auth";
import { requestLogger } from "./middleware/logging";
import { generateId } from "./utils";

export function getUser(id: string): ApiResponse<User | null> {
  const user = findUserById(id);
  if (!user) return formatResponse(null, 404);
  return formatResponse(user);
}

export function createUser(data: { username: string; email: string }): ApiResponse<User> {
  const user: User = {
    id: generateId(),
    username: sanitizeInput(data.username),
    email: sanitizeInput(data.email),
    role: "user",
  };
  insertUser(user);
  return formatResponse(user, 201);
}

export function login(email: string, password: string) {
  const token = authenticate(email, password);
  if (!token) return formatResponse({ error: "invalid credentials" }, 401);
  return formatResponse({ token });
}
