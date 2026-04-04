/** Role-based permission checks. */

import type { User, UserRole } from "../types";

const ROLE_HIERARCHY: Record<UserRole, number> = {
  guest: 0,
  user: 1,
  admin: 2,
};

export function hasPermission(user: User, requiredRole: UserRole): boolean {
  return ROLE_HIERARCHY[user.role] >= ROLE_HIERARCHY[requiredRole];
}

export function isAdmin(user: User): boolean {
  return user.role === "admin";
}
