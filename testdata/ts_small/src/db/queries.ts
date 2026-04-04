/** Database query helpers. */

import { getConnection } from "./connection";
import type { User } from "../types";

export function findUserById(id: string): User | null {
  const conn = getConnection();
  return conn.query("SELECT * FROM users WHERE id = ?", [id]);
}

export function findUserByEmail(email: string): User | null {
  const conn = getConnection();
  return conn.query("SELECT * FROM users WHERE email = ?", [email]);
}

export function insertUser(user: User): void {
  const conn = getConnection();
  conn.execute("INSERT INTO users VALUES (?, ?, ?, ?)", [
    user.id,
    user.username,
    user.email,
    user.role,
  ]);
}
