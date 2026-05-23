import { describe, it, expect } from "vitest";
import { planDroppedTree, type DroppedFile } from "./folder-drop";

// planDroppedTree shouldn't depend on browser APIs — pure data transformation.
describe("planDroppedTree", () => {
  function df(path: string): DroppedFile {
    return { file: new File([], path.split("/").pop() ?? ""), relativePath: path };
  }

  it("flat list — empty directories, one bucket for root", () => {
    const result = planDroppedTree([df("a.txt"), df("b.txt")]);
    expect(result.directories).toEqual([]);
    expect(result.filesByDir.size).toBe(1);
    expect(result.filesByDir.get("")?.length).toBe(2);
  });

  it("one-level nesting groups files under their dir", () => {
    const result = planDroppedTree([df("docs/a.txt"), df("docs/b.txt")]);
    expect(result.directories).toEqual(["docs"]);
    expect(result.filesByDir.get("docs")?.length).toBe(2);
  });

  it("creates every intermediate directory", () => {
    const result = planDroppedTree([df("a/b/c/file.txt")]);
    expect(result.directories).toEqual(["a", "a/b", "a/b/c"]);
  });

  it("directories sorted by depth (parents first)", () => {
    const result = planDroppedTree([
      df("z/deep/file.txt"),
      df("a/file.txt"),
      df("z/file.txt"),
    ]);
    // depth-1 directories should appear before depth-2.
    const depth = (p: string) => p.split("/").length;
    for (let i = 1; i < result.directories.length; i++) {
      expect(depth(result.directories[i])).toBeGreaterThanOrEqual(depth(result.directories[i - 1]));
    }
  });

  it("multiple top-level dirs both seen", () => {
    const result = planDroppedTree([df("a/x.txt"), df("b/y.txt")]);
    expect(result.directories.sort()).toEqual(["a", "b"]);
  });

  it("empty input → empty result", () => {
    const result = planDroppedTree([]);
    expect(result.directories).toEqual([]);
    expect(result.filesByDir.size).toBe(0);
  });
});
