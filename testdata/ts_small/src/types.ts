/** Core type definitions shared across the application. */

export interface User {
  id: string;
  username: string;
  email: string;
  role: UserRole;
}

export type UserRole = "admin" | "user" | "guest";

export interface ApiResponse<T> {
  data: T;
  status: number;
  message?: string;
}

export interface DbConfig {
  host: string;
  port: number;
  database: string;
}

export interface AuthToken {
  token: string;
  expiresAt: number;
}
