/** Tests for user routes. */

import { userRoutes } from "../src/routes/users";

describe("user routes", () => {
  test("create user returns 201", () => {
    const req = { method: "POST", body: { name: "Alice", email: "a@b.com" } };
    const res = { status: () => res, json: () => {} };
    const result = userRoutes(req, res);
    expect(result.status).toBe(201);
  });
});
