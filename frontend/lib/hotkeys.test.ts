// Tests for lib/hotkeys.matches() — the chord-resolution core. Doesn't
// need to install the global listener for these to work; we just feed
// synthetic KeyboardEvents.

import { describe, expect, it } from "vitest";
import { matches, register } from "./hotkeys";

// Helper: build a KeyboardEvent with the modifier flags we want.
function kb(key: string, opts: { metaKey?: boolean; ctrlKey?: boolean; shiftKey?: boolean; altKey?: boolean } = {}) {
  return new KeyboardEvent("keydown", {
    key,
    metaKey: opts.metaKey,
    ctrlKey: opts.ctrlKey,
    shiftKey: opts.shiftKey,
    altKey: opts.altKey,
  });
}

describe("matches", () => {
  it("bare key matches", () => {
    expect(matches("f2", kb("F2"))).toBe(true);
    expect(matches("esc", kb("Escape"))).toBe(true);
    expect(matches("/", kb("/"))).toBe(true);
  });

  it("modifier required but absent fails", () => {
    expect(matches("shift+?", kb("?"))).toBe(false);
  });

  it("modifier present but not required fails", () => {
    expect(matches("k", kb("k", { ctrlKey: true }))).toBe(false);
  });

  it("ctrl+k", () => {
    expect(matches("ctrl+k", kb("k", { ctrlKey: true }))).toBe(true);
    expect(matches("ctrl+k", kb("k", { metaKey: true }))).toBe(false);
  });

  it("multi-modifier", () => {
    expect(
      matches("ctrl+shift+p", kb("p", { ctrlKey: true, shiftKey: true })),
    ).toBe(true);
    expect(
      matches("ctrl+shift+p", kb("p", { ctrlKey: true })),
    ).toBe(false);
  });

  it("case insensitive", () => {
    expect(matches("CTRL+K", kb("K", { ctrlKey: true }))).toBe(true);
    expect(matches("ctrl+k", kb("K", { ctrlKey: true }))).toBe(true);
  });

  it("'esc' alias matches Escape key", () => {
    expect(matches("esc", kb("Escape"))).toBe(true);
  });

  it("alt+f vs option+f are equivalent", () => {
    expect(matches("alt+f", kb("f", { altKey: true }))).toBe(true);
    expect(matches("option+f", kb("f", { altKey: true }))).toBe(true);
  });
});

describe("register", () => {
  it("returns an unregister fn", () => {
    const unregister = register("f1", () => {});
    expect(typeof unregister).toBe("function");
    unregister();
  });

  it("multiple registrations don't crash", () => {
    const a = register("a", () => {});
    const b = register("b", () => {});
    a();
    b();
  });
});
