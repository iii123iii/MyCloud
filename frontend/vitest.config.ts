// Vitest configuration for the MyCloud frontend.
//
// jsdom environment because most of what we test is browser-side
// (localStorage, fetch interception, React hooks). Tests that don't need
// a DOM can still run here — jsdom is cheap.
//
// `globals: true` lets test files use `describe`/`it`/`expect` without
// imports — same ergonomics as Jest. We still import where the IDE needs
// the type but the runtime resolves them.

import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./test/setup.ts"],
    include: [
      "test/**/*.test.{ts,tsx}",
      "lib/**/*.test.{ts,tsx}",
      "components/**/*.test.{ts,tsx}",
      "hooks/**/*.test.{ts,tsx}",
    ],
  },
});
