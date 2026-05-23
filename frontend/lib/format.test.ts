// Tests for lib/format. Display helpers used throughout the UI for file
// sizes, dates, MIME labels. Bugs here are visible — bad bytes formatting
// = "0.0 KB" for everything; bad date parsing = "Invalid Date" stickers.

import { describe, expect, it } from "vitest";
import {
  formatBytes,
  parseServerDate,
  mimeToLabel,
  mimeToColor,
} from "./format";

describe("formatBytes", () => {
  it("zero is 0 B", () => {
    expect(formatBytes(0)).toBe("0 B");
  });

  it("negative is 0 B", () => {
    expect(formatBytes(-100)).toBe("0 B");
  });

  it("bytes under 1KB", () => {
    expect(formatBytes(512)).toBe("512 B");
  });

  it("KB rounding", () => {
    // 1500 bytes ≈ 1.5 KB
    expect(formatBytes(1500)).toBe("1.5 KB");
  });

  it("MB rounding", () => {
    // 1.5 MB
    expect(formatBytes(1500 * 1024)).toBe("1.5 MB");
  });

  it("GB", () => {
    const oneGB = 1024 * 1024 * 1024;
    expect(formatBytes(oneGB)).toBe("1.0 GB");
  });

  it("TB", () => {
    const oneTB = 1024 * 1024 * 1024 * 1024;
    expect(formatBytes(oneTB)).toBe("1.0 TB");
  });

  it("clamps to TB for absurdly large values", () => {
    const huge = Math.pow(1024, 6); // exabytes — should still show as TB
    expect(formatBytes(huge)).toMatch(/TB$/);
  });
});

describe("parseServerDate", () => {
  it("parses MariaDB DATETIME (space-separated, no zone) as UTC", () => {
    const d = parseServerDate("2026-05-22 10:30:00");
    expect(d.getTime()).not.toBeNaN();
    // Should resolve to the same UTC instant as the ISO equivalent.
    expect(d.toISOString()).toBe("2026-05-22T10:30:00.000Z");
  });

  it("parses ISO with Z unchanged", () => {
    const d = parseServerDate("2026-05-22T10:30:00Z");
    expect(d.toISOString()).toBe("2026-05-22T10:30:00.000Z");
  });

  it("parses ISO with milliseconds", () => {
    const d = parseServerDate("2026-05-22T10:30:00.123Z");
    expect(d.toISOString()).toBe("2026-05-22T10:30:00.123Z");
  });

  it("parses ISO with offset zone", () => {
    const d = parseServerDate("2026-05-22T10:30:00+02:00");
    expect(d.getTime()).not.toBeNaN();
  });

  it("invalid string yields Invalid Date", () => {
    const d = parseServerDate("not a date");
    expect(isNaN(d.getTime())).toBe(true);
  });
});

describe("mimeToLabel", () => {
  it("known specific types", () => {
    expect(mimeToLabel("application/pdf")).toBe("PDF");
    expect(mimeToLabel("application/json")).toBe("JSON");
    expect(mimeToLabel("text/markdown")).toBe("Markdown");
    expect(mimeToLabel("text/csv")).toBe("CSV");
  });

  it("type-family fallbacks", () => {
    expect(mimeToLabel("image/jpeg")).toBe("Image");
    expect(mimeToLabel("video/mp4")).toBe("Video");
    expect(mimeToLabel("audio/mpeg")).toBe("Audio");
    expect(mimeToLabel("text/plain")).toBe("Text");
  });

  it("MS Office variants", () => {
    expect(mimeToLabel("application/zip")).toBe("Archive");
    expect(mimeToLabel("application/vnd.openxmlformats-officedocument.wordprocessingml.document")).toBe("Word");
    expect(mimeToLabel("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")).toBe("Spreadsheet");
    expect(mimeToLabel("application/vnd.openxmlformats-officedocument.presentationml.presentation")).toBe("Slides");
  });

  it("unknown MIME falls back to 'File'", () => {
    expect(mimeToLabel("application/x-fake-binary-format")).toBe("File");
    expect(mimeToLabel("")).toBe("File");
  });
});

describe("mimeToColor", () => {
  it("images get emerald", () => {
    expect(mimeToColor("image/jpeg")).toContain("emerald");
  });

  it("videos get violet", () => {
    expect(mimeToColor("video/mp4")).toContain("violet");
  });

  it("PDFs get red", () => {
    expect(mimeToColor("application/pdf")).toContain("red");
  });

  it("unknown gets muted-foreground", () => {
    expect(mimeToColor("application/unknown")).toContain("muted-foreground");
  });
});
