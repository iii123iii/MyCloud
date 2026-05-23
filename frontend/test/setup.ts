// Vitest setup: extends `expect` with @testing-library/jest-dom matchers
// (toBeInTheDocument, toHaveTextContent, etc.) and patches a minimal
// localStorage if jsdom didn't provide one.

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach } from "vitest";

// Reset localStorage between tests so token state from one doesn't leak
// into another. jsdom provides a real Storage instance; this just clears
// it.
beforeEach(() => {
  if (typeof localStorage !== "undefined") {
    localStorage.clear();
  }
});

afterEach(() => {
  // Restore any global mocks set by individual tests.
  if (typeof localStorage !== "undefined") {
    localStorage.clear();
  }
});
