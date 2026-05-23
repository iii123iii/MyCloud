// Tests for lib/folder-drop. planDroppedTree turns a flat list of
// (file, relativePath) pairs into a plan the upload queue executes:
//   - directories: list of dirs to create, parents first
//   - filesByDir : map of dir path → DroppedFile[]

import { describe, expect, it } from "vitest";
import { planDroppedTree, type DroppedFile } from "./folder-drop";

function dropped(path: string): DroppedFile {
  return {
    file: new File(["x"], path.split("/").pop() ?? "f.txt"),
    relativePath: path,
  };
}

describe("planDroppedTree", () => {
  it("flat files at root produce no directories", () => {
    const plan = planDroppedTree([dropped("a.txt"), dropped("b.txt")]);
    expect(plan.directories).toEqual([]);
    const rootFiles = plan.filesByDir.get("") ?? [];
    expect(rootFiles).toHaveLength(2);
  });

  it("one nested folder produces one directory", () => {
    const plan = planDroppedTree([dropped("photos/a.jpg"), dropped("photos/b.jpg")]);
    expect(plan.directories).toEqual(["photos"]);
    const photos = plan.filesByDir.get("photos") ?? [];
    expect(photos).toHaveLength(2);
  });

  it("deeply nested folders ordered parent-first by depth", () => {
    const plan = planDroppedTree([dropped("a/b/c/file.txt")]);
    expect(plan.directories).toEqual(["a", "a/b", "a/b/c"]);
    // File lives under the deepest path.
    expect(plan.filesByDir.get("a/b/c")).toHaveLength(1);
  });

  it("shared ancestors dedup across siblings", () => {
    const plan = planDroppedTree([
      dropped("a/b/x.txt"),
      dropped("a/b/y.txt"),
      dropped("a/c/z.txt"),
    ]);
    expect(plan.directories.sort()).toEqual(["a", "a/b", "a/c"]);
    expect(plan.filesByDir.get("a/b")).toHaveLength(2);
    expect(plan.filesByDir.get("a/c")).toHaveLength(1);
  });

  it("mixed depths: root file alongside nested", () => {
    const plan = planDroppedTree([dropped("root.txt"), dropped("sub/nested.txt")]);
    expect(plan.directories).toEqual(["sub"]);
    expect(plan.filesByDir.get("")).toHaveLength(1);
    expect(plan.filesByDir.get("sub")).toHaveLength(1);
  });

  it("empty input returns empty plan", () => {
    const plan = planDroppedTree([]);
    expect(plan.directories).toEqual([]);
    expect(plan.filesByDir.size).toBe(0);
  });
});
