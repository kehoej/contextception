/** Authentication middleware. */

import { verify } from "jsonwebtoken";
import { hasPermission } from "../auth/permissions";
import type { User, UserRole } from "../types";

export function requireAuth(handler: Function) {
  return (req: any, res: any) => {
    if (!req.user) {
      return res.status(401).json({ error: "unauthorized" });
    }
    return handler(req, res);
  };
}

export function requireRole(role: UserRole) {
  return (handler: Function) => {
    return (req: any, res: any) => {
      if (!req.user || !hasPermission(req.user as User, role)) {
        return res.status(403).json({ error: "forbidden" });
      }
      return handler(req, res);
    };
  };
}
