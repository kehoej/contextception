/** Shared utility functions. */

import { nanoid } from "nanoid";
import type { ApiResponse } from "./types";

export function formatResponse<T>(data: T, status = 200): ApiResponse<T> {
  return { data, status };
}

export function validateEmail(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

export function generateId(): string {
  return Math.random().toString(36).slice(2, 10);
}
