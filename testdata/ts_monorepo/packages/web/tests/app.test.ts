/** Tests for the web app. */

import { App } from "../src/app";

describe("App", () => {
  test("creates app with pages", () => {
    const app = App();
    expect(app.pages.length).toBeGreaterThan(0);
  });
});
