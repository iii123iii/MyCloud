import { describe, it, expect } from "vitest";
import {
  isTextBased,
  isEditableImage,
  isEditableText,
  getEditMode,
  MIME_TO_LANG,
} from "./file-kind";

describe("isTextBased", () => {
  it("recognises text/* MIMEs", () => {
    expect(isTextBased("text/plain")).toBe(true);
    expect(isTextBased("text/html")).toBe(true);
    expect(isTextBased("text/x-go")).toBe(true);
  });
  it("recognises common application/* text-shaped MIMEs", () => {
    expect(isTextBased("application/json")).toBe(true);
    expect(isTextBased("application/xml")).toBe(true);
    expect(isTextBased("application/javascript")).toBe(true);
  });
  it("rejects binary types", () => {
    expect(isTextBased("image/png")).toBe(false);
    expect(isTextBased("application/pdf")).toBe(false);
    expect(isTextBased("application/octet-stream")).toBe(false);
  });
  it("falls back to extension for octet-stream", () => {
    expect(isTextBased("application/octet-stream", "notes.md")).toBe(true);
    expect(isTextBased("application/octet-stream", "config.yaml")).toBe(true);
    expect(isTextBased("application/octet-stream", "binary.bin")).toBe(false);
  });
  it("handles missing name gracefully", () => {
    expect(isTextBased("application/octet-stream")).toBe(false);
  });
});

describe("isEditableImage", () => {
  it("accepts raster types", () => {
    expect(isEditableImage("image/png")).toBe(true);
    expect(isEditableImage("image/jpeg")).toBe(true);
    expect(isEditableImage("image/webp")).toBe(true);
  });
  it("rejects SVG (text-shaped) + GIF (no anim)", () => {
    expect(isEditableImage("image/svg+xml")).toBe(false);
    expect(isEditableImage("image/gif")).toBe(false);
  });
});

describe("getEditMode", () => {
  it("returns 'image' for raster images", () => {
    expect(getEditMode({ mime_type: "image/png", name: "a.png" })).toBe("image");
  });
  it("returns 'text' for text/* + JSON/XML/JS", () => {
    expect(getEditMode({ mime_type: "text/plain", name: "a.txt" })).toBe("text");
    expect(getEditMode({ mime_type: "application/json", name: "a.json" })).toBe("text");
  });
  it("returns 'text' for SVG (text-shaped, not raster)", () => {
    expect(getEditMode({ mime_type: "image/svg+xml", name: "a.svg" })).toBe(null);
    // SVG isn't in isTextBased — should fall through to null.
  });
  it("returns null for non-editable types", () => {
    expect(getEditMode({ mime_type: "application/pdf", name: "a.pdf" })).toBe(null);
    expect(getEditMode({ mime_type: "video/mp4", name: "a.mp4" })).toBe(null);
  });
  it("returns 'text' for unknown MIME with text extension", () => {
    expect(getEditMode({ mime_type: "application/octet-stream", name: "config.yaml" })).toBe("text");
  });
});

describe("MIME_TO_LANG", () => {
  it("maps common languages", () => {
    expect(MIME_TO_LANG["text/javascript"]).toBe("javascript");
    expect(MIME_TO_LANG["text/x-python"]).toBe("python");
    expect(MIME_TO_LANG["application/json"]).toBe("json");
    expect(MIME_TO_LANG["text/x-go"]).toBe("go");
  });
});

describe("isEditableText alias", () => {
  it("matches isTextBased", () => {
    expect(isEditableText("text/plain")).toBe(true);
    expect(isEditableText("application/pdf")).toBe(false);
  });
});
