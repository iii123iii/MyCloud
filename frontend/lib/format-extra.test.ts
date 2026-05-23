import { describe, it, expect } from "vitest";
import {
  formatBytes,
  formatDate,
  formatRelative,
  parseServerDate,
  mimeToLabel,
  mimeToColor,
  PREVIEWABLE_APPLICATION_TYPES,
  PREVIEWABLE_SPREADSHEET_TYPES,
  PREVIEWABLE_WORD_TYPES,
} from "./format";

describe("formatBytes", () => {
  it("returns 0 B for zero / negative", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(-1)).toBe("0 B");
  });
  it("uses bytes when under 1 KB", () => {
    expect(formatBytes(500)).toBe("500 B");
  });
  it("scales to KB / MB / GB", () => {
    expect(formatBytes(1024)).toMatch(/1\.0 KB/);
    expect(formatBytes(1024 * 1024)).toMatch(/1\.0 MB/);
    expect(formatBytes(1024 * 1024 * 1024)).toMatch(/1\.0 GB/);
  });
  it("caps at TB", () => {
    const huge = 1024 * 1024 * 1024 * 1024 * 1024; // 1 PB
    expect(formatBytes(huge)).toMatch(/TB/);
  });
});

describe("parseServerDate", () => {
  it("parses MariaDB DATETIME format as UTC", () => {
    const d = parseServerDate("2024-01-15 12:34:56");
    expect(d.getUTCFullYear()).toBe(2024);
    expect(d.getUTCMonth()).toBe(0);
    expect(d.getUTCDate()).toBe(15);
    expect(d.getUTCHours()).toBe(12);
  });
  it("preserves ISO-8601 with Z suffix", () => {
    const d = parseServerDate("2024-01-15T12:34:56Z");
    expect(d.getUTCHours()).toBe(12);
  });
  it("preserves timezone-suffixed strings", () => {
    const d = parseServerDate("2024-01-15T12:00:00+05:00");
    // 12:00 in +05 → 07:00 UTC
    expect(d.getUTCHours()).toBe(7);
  });
});

describe("formatDate / formatRelative", () => {
  it("returns the input on invalid date", () => {
    expect(formatDate("not a date")).toBe("not a date");
    expect(formatRelative("not a date")).toBe("not a date");
  });
  it("formats a valid date", () => {
    const out = formatDate("2024-01-15 12:34:56");
    expect(out).toMatch(/2024/);
    expect(out).toMatch(/Jan/);
  });
});

describe("mimeToLabel", () => {
  const cases: Array<[string, string]> = [
    ["image/png", "Image"],
    ["video/mp4", "Video"],
    ["audio/mpeg", "Audio"],
    ["application/pdf", "PDF"],
    ["application/json", "JSON"],
    ["application/xml", "XML"],
    ["application/javascript", "JavaScript"],
    ["text/yaml", "YAML"],
    ["text/csv", "CSV"],
    ["text/markdown", "Markdown"],
    ["text/plain", "Text"],
    ["application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "Spreadsheet"],
    ["application/vnd.openxmlformats-officedocument.wordprocessingml.document", "Word"],
    ["application/zip", "Archive"],
    ["application/x-msword", "Word"],
    ["unknown/whatever", "File"],
  ];
  for (const [mime, label] of cases) {
    it(`maps ${mime} → ${label}`, () => {
      expect(mimeToLabel(mime)).toBe(label);
    });
  }
});

describe("mimeToColor", () => {
  it("assigns distinct colors to media types", () => {
    expect(mimeToColor("image/png")).toContain("emerald");
    expect(mimeToColor("video/mp4")).toContain("violet");
    expect(mimeToColor("audio/mpeg")).toContain("pink");
    expect(mimeToColor("application/pdf")).toContain("red");
    expect(mimeToColor("text/plain")).toContain("sky");
    expect(mimeToColor("unknown/whatever")).toContain("muted");
  });
});

describe("Previewable type sets", () => {
  it("application set contains JSON + XML + JS family", () => {
    expect(PREVIEWABLE_APPLICATION_TYPES.has("application/json")).toBe(true);
    expect(PREVIEWABLE_APPLICATION_TYPES.has("application/xml")).toBe(true);
    expect(PREVIEWABLE_APPLICATION_TYPES.has("application/javascript")).toBe(true);
  });
  it("spreadsheet set covers xlsx / xls / ods", () => {
    expect(
      PREVIEWABLE_SPREADSHEET_TYPES.has(
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      ),
    ).toBe(true);
    expect(PREVIEWABLE_SPREADSHEET_TYPES.has("application/vnd.ms-excel")).toBe(true);
  });
  it("word set covers docx + doc", () => {
    expect(
      PREVIEWABLE_WORD_TYPES.has(
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      ),
    ).toBe(true);
    expect(PREVIEWABLE_WORD_TYPES.has("application/msword")).toBe(true);
  });
});
