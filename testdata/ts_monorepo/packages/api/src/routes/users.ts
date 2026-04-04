/** User API routes. */

import { z } from "zod";
import { formatResponse, validateEmail, generateId } from "@myapp/shared";
import type { User } from "@myapp/shared";
import { NotFoundError, ValidationError } from "@myapp/shared/errors";

const users: User[] = [];

export function userRoutes(req: any, res: any) {
  // GET /users/:id
  if (req.method === "GET") {
    const user = users.find((u) => u.id === req.params.id);
    if (!user) throw new NotFoundError("User");
    return formatResponse(user);
  }

  // POST /users
  if (req.method === "POST") {
    if (!validateEmail(req.body.email)) {
      throw new ValidationError("Invalid email");
    }
    const user: User = {
      id: generateId(),
      name: req.body.name,
      email: req.body.email,
    };
    users.push(user);
    return formatResponse(user, 201);
  }
}
