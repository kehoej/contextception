/** Shared utility functions. */

import type { ApiResponse } from "./types";

export function formatResponse<T>(data: T, status = 200): ApiResponse<T> {
  return { data, status };
}

export function sanitizeInput(input: string): string {
  return input.trim().replace(/[<>]/g, "");
}

export function generateId(): string {
  return Math.random().toString(36).slice(2);
}
