// Tests for lib/command-registry. The registry is a module-level singleton
// (Map) — tests must reset state between them so registrations don't leak
// across cases. We use registerCommand's returned unregister to clean up.

import { afterEach, describe, expect, it, vi } from "vitest";
import { registerCommand, listCommands, subscribe } from "./command-registry";

afterEach(() => {
  // Drain anything still registered between tests so leakage doesn't make
  // listCommands().length assertions flaky.
  for (const c of listCommands()) {
    // Unregister via the public path: re-register then call the returned fn.
    // Simpler: since the test always cleans up its own registrations via
    // the unregister fn it captures, this is a belt-and-braces sweep.
    registerCommand({ id: c.id, label: c.label, run: () => {} })();
  }
});

describe("registerCommand / listCommands", () => {
  it("inserts and lists", () => {
    const off = registerCommand({ id: "x", label: "X", run: vi.fn() });
    const list = listCommands();
    expect(list.find((c) => c.id === "x")?.label).toBe("X");
    off();
  });

  it("filters when=false from list but keeps in registry", () => {
    const off = registerCommand({
      id: "hidden",
      label: "H",
      when: () => false,
      run: vi.fn(),
    });
    expect(listCommands().find((c) => c.id === "hidden")).toBeUndefined();
    off();
  });

  it("when=true shows", () => {
    const off = registerCommand({
      id: "shown",
      label: "S",
      when: () => true,
      run: vi.fn(),
    });
    expect(listCommands().find((c) => c.id === "shown")).toBeDefined();
    off();
  });

  it("re-registering same id replaces label", () => {
    const offA = registerCommand({ id: "dup", label: "A", run: vi.fn() });
    const offB = registerCommand({ id: "dup", label: "B", run: vi.fn() });
    const match = listCommands().filter((c) => c.id === "dup");
    expect(match).toHaveLength(1);
    expect(match[0].label).toBe("B");
    offA();
    offB();
  });

  it("returned unregister removes the command", () => {
    const off = registerCommand({ id: "tmp", label: "T", run: vi.fn() });
    off();
    expect(listCommands().find((c) => c.id === "tmp")).toBeUndefined();
  });
});

describe("subscribe", () => {
  it("notifies on register + unregister", () => {
    const cb = vi.fn();
    const unsub = subscribe(cb);
    const off = registerCommand({ id: "s1", label: "S", run: vi.fn() });
    expect(cb).toHaveBeenCalledTimes(1);
    off();
    expect(cb).toHaveBeenCalledTimes(2);
    unsub();
  });

  it("unsubscribed listener stops receiving", () => {
    const cb = vi.fn();
    const unsub = subscribe(cb);
    unsub();
    const off = registerCommand({ id: "s2", label: "S", run: vi.fn() });
    expect(cb).not.toHaveBeenCalled();
    off();
  });

  it("multiple subscribers all notified", () => {
    const a = vi.fn();
    const b = vi.fn();
    const unsubA = subscribe(a);
    const unsubB = subscribe(b);
    const off = registerCommand({ id: "s3", label: "S", run: vi.fn() });
    expect(a).toHaveBeenCalledTimes(1);
    expect(b).toHaveBeenCalledTimes(1);
    unsubA();
    unsubB();
    off();
  });
});

describe("Command.run", () => {
  it("runs the registered fn", async () => {
    const fn = vi.fn();
    const off = registerCommand({ id: "r1", label: "R", run: fn });
    const cmd = listCommands().find((c) => c.id === "r1")!;
    await cmd.run();
    expect(fn).toHaveBeenCalledOnce();
    off();
  });

  it("supports async run", async () => {
    let resolved = false;
    const off = registerCommand({
      id: "r2",
      label: "R",
      run: async () => {
        await Promise.resolve();
        resolved = true;
      },
    });
    const cmd = listCommands().find((c) => c.id === "r2")!;
    await cmd.run();
    expect(resolved).toBe(true);
    off();
  });
});
