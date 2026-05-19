"use client";

import { forwardRef, useImperativeHandle, useState } from "react";
import CodeMirror, { type Extension } from "@uiw/react-codemirror";

import { cpp } from "@codemirror/lang-cpp";
import { css } from "@codemirror/lang-css";
import { html } from "@codemirror/lang-html";
import { java } from "@codemirror/lang-java";
import { javascript } from "@codemirror/lang-javascript";
import { json } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { php } from "@codemirror/lang-php";
import { python } from "@codemirror/lang-python";
import { rust } from "@codemirror/lang-rust";
import { sql } from "@codemirror/lang-sql";
import { xml } from "@codemirror/lang-xml";
import { yaml } from "@codemirror/lang-yaml";

import { MIME_TO_LANG } from "@/lib/file-kind";

export interface TextEditorRef {
  /**
   * Encode the current buffer back into a blob using the original file's MIME
   * type. Re-uploading this blob with the same name in the same folder
   * triggers the backend's versioning path.
   */
  getOutputBlob(mime: string): Promise<Blob>;
}

interface Props {
  /** Initial buffer contents. */
  initialText: string;
  /** Original file MIME — picks CodeMirror's syntax highlight extension. */
  mime: string;
  /** Optional fired on every keystroke so the parent can show "Unsaved". */
  onDirty?: () => void;
}

/**
 * Map our internal language id (from MIME_TO_LANG) → a CodeMirror extension.
 * Languages we don't have a pack for fall back to plain text, which still
 * gets line numbers, search, undo, etc. from `basicSetup`.
 */
function extensionForLang(lang: string | undefined): Extension[] {
  switch (lang) {
    case "javascript":   return [javascript({ jsx: true, typescript: false })];
    case "typescript":   return [javascript({ jsx: true, typescript: true })];
    case "json":         return [json()];
    case "markdown":     return [markdown()];
    case "yaml":         return [yaml()];
    case "html":         return [html()];
    case "css":          return [css()];
    case "xml":          return [xml()];
    case "python":       return [python()];
    case "java":         return [java()];
    case "cpp":
    case "c":            return [cpp()];
    case "csharp":       return [java()]; // close enough syntactically, no official csharp pack
    case "rust":         return [rust()];
    case "php":          return [php()];
    case "sql":          return [sql()];
    default:             return [];
  }
}

export const TextEditor = forwardRef<TextEditorRef, Props>(
  function TextEditor({ initialText, mime, onDirty }, ref) {
    const [value, setValue] = useState(initialText);
    const langExts = extensionForLang(MIME_TO_LANG[mime]);

    useImperativeHandle(
      ref,
      () => ({
        async getOutputBlob(outputMime: string) {
          return new Blob([value], { type: outputMime });
        },
      }),
      [value],
    );

    return (
      <div className="h-full w-full overflow-hidden">
        <CodeMirror
          value={value}
          height="100%"
          theme="dark"
          extensions={langExts}
          onChange={(next) => {
            setValue(next);
            onDirty?.();
          }}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            highlightActiveLine: true,
            autocompletion: true,
            bracketMatching: true,
            closeBrackets: true,
          }}
          className="h-full text-sm"
        />
      </div>
    );
  },
);
