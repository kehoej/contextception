/** Tests for route handlers. */

import { getUser, createUser, login } from "../src/routes";

describe("routes", () => {
  test("getUser returns 404 for unknown id", () => {
    const result = getUser("unknown");
    expect(result.status).toBe(404);
  });

  test("createUser returns 201", () => {
    const result = createUser({ username: "alice", email: "a@test.com" });
    expect(result.status).toBe(201);
  });

  test("login returns token on success", () => {
    const result = login("alice@test.com", "password123");
    expect(result.data).toBeDefined();
  });
});
