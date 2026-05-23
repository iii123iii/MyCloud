// Tests for lib/file-kind. The editor uses getEditMode to decide between
// the image editor (tldraw), the text editor (CodeMirror), or no editor.

import { describe, expect, it } from "vitest";
import { getEditMode, isEditableImage, isEditableText, isTextBased } from "./file-kind";

const file = (mime_type: string, name = "f") => ({ mime_type, name });

describe("getEditMode", () => {
  it("PNG → image editor", () => {
    expect(getEditMode(file("image/png", "a.png"))).toBe("image");
  });

  it("JPEG → image editor", () => {
    expect(getEditMode(file("image/jpeg", "a.jpg"))).toBe("image");
  });

  it("SVG is NOT image-editable (text-based; treated as text)", () => {
    // SVG isn't in ANNOTATABLE_IMAGE_TYPES — it's text/xml under the hood
    // and the text editor handles it better than tldraw.
    expect(getEditMode(file("image/svg+xml", "a.svg"))).not.toBe("image");
  });

  it("GIF is NOT editable (annotation overlay can't animate)", () => {
    expect(getEditMode(file("image/gif", "a.gif"))).toBeNull();
  });

  it("plain text → text editor", () => {
    expect(getEditMode(file("text/plain", "a.txt"))).toBe("text");
  });

  it("markdown → text editor", () => {
    expect(getEditMode(file("text/markdown", "a.md"))).toBe("text");
  });

  it("JSON → text editor", () => {
    expect(getEditMode(file("application/json", "a.json"))).toBe("text");
  });

  it("source code .go (extension fallback when MIME is generic)", () => {
    expect(getEditMode(file("application/octet-stream", "main.go"))).toBe("text");
  });

  it("source code .py (extension fallback)", () => {
    expect(getEditMode(file("application/octet-stream", "script.py"))).toBe("text");
  });

  it("source code .tsx (extension fallback)", () => {
    expect(getEditMode(file("application/octet-stream", "App.tsx"))).toBe("text");
  });

  it("PDF → null", () => {
    expect(getEditMode(file("application/pdf", "doc.pdf"))).toBeNull();
  });

  it("video → null", () => {
    expect(getEditMode(file("video/mp4", "v.mp4"))).toBeNull();
  });

  it("zip → null", () => {
    expect(getEditMode(file("application/zip", "a.zip"))).toBeNull();
  });

  it("unknown binary with no extension hint → null", () => {
    expect(getEditMode(file("application/octet-stream", "noext"))).toBeNull();
  });
});

describe("isEditableImage", () => {
  it("returns true only for ANNOTATABLE_IMAGE_TYPES set", () => {
    expect(isEditableImage("image/png")).toBe(true);
    expect(isEditableImage("image/jpeg")).toBe(true);
    expect(isEditableImage("image/webp")).toBe(true);
    expect(isEditableImage("image/svg+xml")).toBe(false);
    expect(isEditableImage("image/gif")).toBe(false);
    expect(isEditableImage("application/pdf")).toBe(false);
  });
});

describe("isEditableText / isTextBased", () => {
  it("text/* is editable", () => {
    expect(isEditableText("text/anything")).toBe(true);
    expect(isTextBased("text/anything")).toBe(true);
  });

  it("application/octet-stream + text extension is editable", () => {
    expect(isEditableText("application/octet-stream", "log.txt")).toBe(true);
    expect(isEditableText("application/octet-stream", "tsconfig.json")).toBe(true);
  });

  it("application/octet-stream WITHOUT name is not editable", () => {
    expect(isEditableText("application/octet-stream")).toBe(false);
  });

  it("binary types are not text-based", () => {
    expect(isEditableText("image/jpeg")).toBe(false);
    expect(isTextBased("application/zip")).toBe(false);
  });
});
