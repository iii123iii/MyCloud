import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { register as registerHotkey } from "./hotkeys";

describe("registerHotkey", () => {
  beforeEach(() => {
    // Reset by reloading the module would be cleaner, but we just register
    // fresh handlers per test and unregister via the returned dispose fn.
  });

  it("fires on a single-key chord", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("k", fn);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k" }));
    expect(fn).toHaveBeenCalledTimes(1);
    dispose();
  });

  it("fires on mod+key on non-mac", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("mod+k", fn);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true }));
    expect(fn).toHaveBeenCalledTimes(1);
    dispose();
  });

  it("ignores plain k when mod+k registered", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("mod+k", fn);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k" }));
    expect(fn).not.toHaveBeenCalled();
    dispose();
  });

  it("skips keydown inside <input>", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("k", fn);
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "k", bubbles: true }));
    expect(fn).not.toHaveBeenCalled();
    document.body.removeChild(input);
    dispose();
  });

  it("skips keydown within data-no-hotkeys subtree", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("k", fn);
    const div = document.createElement("div");
    div.setAttribute("data-no-hotkeys", "");
    const child = document.createElement("div");
    div.appendChild(child);
    document.body.appendChild(div);
    child.dispatchEvent(new KeyboardEvent("keydown", { key: "k", bubbles: true }));
    expect(fn).not.toHaveBeenCalled();
    document.body.removeChild(div);
    dispose();
  });

  it("supports shift+? chord", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("shift+?", fn);
    window.dispatchEvent(
      new KeyboardEvent("keydown", { key: "?", shiftKey: true }),
    );
    expect(fn).toHaveBeenCalledTimes(1);
    dispose();
  });

  it("dispose stops further firings", () => {
    const fn = vi.fn();
    const dispose = registerHotkey("x", fn);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "x" }));
    expect(fn).toHaveBeenCalledTimes(1);
    dispose();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "x" }));
    expect(fn).toHaveBeenCalledTimes(1); // still 1
  });
});
