/**
 * Shared MIME / extension classification helpers.
 *
 * Centralises the logic that PreviewModal, EditFileDialog, FileContextMenu and
 * the editor components used to duplicate inline. Anything that branches on
 * "what kind of file is this?" should funnel through here.
 */

import { PREVIEWABLE_APPLICATION_TYPES } from "./format";

// ─── MIME → language id (used by Shiki for read-only highlight and CodeMirror
//      for editable highlight) ────────────────────────────────────────────────

export const MIME_TO_LANG: Record<string, string> = {
  "text/javascript":           "javascript",
  "text/typescript":           "typescript",
  "text/html":                 "html",
  "text/css":                  "css",
  "text/xml":                  "xml",
  "text/markdown":             "markdown",
  "text/x-markdown":           "markdown",
  "text/yaml":                 "yaml",
  "text/x-yaml":               "yaml",
  "text/x-python":             "python",
  "text/x-java":               "java",
  "text/x-c":                  "c",
  "text/x-c++src":             "cpp",
  "text/x-csharp":             "csharp",
  "text/x-go":                 "go",
  "text/x-rust":               "rust",
  "text/x-sh":                 "shellscript",
  "text/x-shellscript":        "shellscript",
  "text/x-ruby":               "ruby",
  "text/x-php":                "php",
  "text/x-swift":              "swift",
  "text/x-kotlin":             "kotlin",
  "application/json":          "json",
  "application/ld+json":       "json",
  "application/xml":           "xml",
  "application/javascript":    "javascript",
  "application/x-javascript":  "javascript",
  "application/x-yaml":        "yaml",
  "application/x-sh":          "shellscript",
  "application/x-shellscript": "shellscript",
  "application/x-httpd-php":   "php",
  "application/graphql":       "graphql",
};

// Extensions that are plain text but whose MIME the browser doesn't always
// detect correctly (especially when uploaded from desktop sync). Anything in
// this set is editable as text even if the MIME comes through as
// application/octet-stream.
const TEXT_EXTENSIONS = new Set([
  "txt", "md", "markdown", "rst", "log",
  "json", "ndjson", "jsonl",
  "yaml", "yml", "toml", "ini", "env",
  "csv", "tsv",
  "xml", "html", "htm", "css", "scss", "less",
  "js", "jsx", "ts", "tsx", "mjs", "cjs",
  "py", "rb", "go", "rs", "java", "kt", "swift",
  "c", "h", "cpp", "hpp", "cc", "cs",
  "sh", "bash", "zsh", "fish",
  "php", "pl", "lua", "r",
  "sql", "graphql", "gql",
  "vue", "svelte",
  "dockerfile", "makefile",
]);

function extensionOf(name: string | undefined): string {
  if (!name) return "";
  const dot = name.lastIndexOf(".");
  if (dot < 0 || dot === name.length - 1) return "";
  return name.slice(dot + 1).toLowerCase();
}

/** Plain-text-shaped content (covers text/*, JSON/XML/JS family, etc.). */
export function isTextBased(mime: string, name?: string): boolean {
  if (mime.startsWith("text/")) return true;
  if (PREVIEWABLE_APPLICATION_TYPES.has(mime)) return true;
  // Fallback for files uploaded with a generic MIME — e.g. a `.md` posted as
  // application/octet-stream.
  if (mime === "application/octet-stream" && name) {
    return TEXT_EXTENSIONS.has(extensionOf(name));
  }
  return false;
}

// ─── Editor eligibility ──────────────────────────────────────────────────────
//
// tldraw renders raster images cleanly. SVG is text-shaped and would be more
// useful in the text editor than as a raster annotation target. GIF could
// render but the annotation overlay wouldn't animate, so we skip it.

const ANNOTATABLE_IMAGE_TYPES = new Set([
  "image/png",
  "image/jpeg",
  "image/jpg",
  "image/webp",
  "image/bmp",
]);

export function isEditableImage(mime: string): boolean {
  return ANNOTATABLE_IMAGE_TYPES.has(mime);
}

export function isEditableText(mime: string, name?: string): boolean {
  return isTextBased(mime, name);
}

export type EditMode = "image" | "text" | null;

/** Returns the editor mode appropriate for a file, or null if not editable. */
export function getEditMode(file: { mime_type: string; name: string }): EditMode {
  if (isEditableImage(file.mime_type)) return "image";
  if (isEditableText(file.mime_type, file.name)) return "text";
  return null;
}
