// Tests for lib/utils — the cn() Tailwind class merger. Most uses are trivial
// concatenation but the lib/utils module deserves coverage because every
// component depends on it.

import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("merges string classnames", () => {
    expect(cn("a", "b")).toContain("a");
    expect(cn("a", "b")).toContain("b");
  });

  it("dedupes conflicting Tailwind utilities (later wins)", () => {
    // The whole point of tailwind-merge: `px-2` should be replaced by `px-4`.
    const out = cn("px-2", "px-4");
    expect(out).toContain("px-4");
    expect(out).not.toContain("px-2");
  });

  it("handles undefined / null / false inputs", () => {
    const out = cn("base", undefined, null, false, "real");
    expect(out).toContain("base");
    expect(out).toContain("real");
  });

  it("handles array inputs", () => {
    expect(cn(["a", "b"])).toContain("a");
    expect(cn(["a", "b"])).toContain("b");
  });

  it("handles object inputs (conditional)", () => {
    const out = cn({ shown: true, hidden: false });
    expect(out).toContain("shown");
    expect(out).not.toContain("hidden");
  });

  it("returns empty string for empty input", () => {
    expect(cn()).toBe("");
  });
});
