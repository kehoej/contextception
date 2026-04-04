/** Tests for authentication. */

import { authenticate, validateCredentials } from "../src/auth/login";
import { createToken, isTokenValid } from "../src/auth/token";

describe("auth", () => {
  test("validateCredentials rejects short password", () => {
    expect(validateCredentials("a@b.com", "short")).toBe(false);
  });

  test("createToken returns valid token", () => {
    const token = createToken("user1");
    expect(isTokenValid(token)).toBe(true);
  });
});
