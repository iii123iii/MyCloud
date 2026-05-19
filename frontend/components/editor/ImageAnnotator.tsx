"use client";

import { forwardRef, useImperativeHandle, useRef } from "react";
import { Tldraw, type Editor } from "tldraw";

import "tldraw/tldraw.css";

export interface ImageAnnotatorRef {
  /**
   * Flatten the canvas (the original image + any user annotations) into a
   * single raster blob. `format` should match the original file's MIME so the
   * saved version doesn't silently change type.
   */
  getOutputBlob(format: "png" | "jpeg" | "webp"): Promise<Blob | null>;
}

interface Props {
  /** The image file to load as the background. */
  imageFile: File;
}

/**
 * Image annotation surface backed by tldraw v5. The original image is loaded
 * through tldraw's standard "files" external-content handler — same code path
 * as a drag-drop, so we get correct asset sizing + the image shape for free.
 *
 * The user gets tldraw's full toolbar (pen, eraser, shapes, text, arrows,
 * colours, undo / redo) — no custom UI to maintain.
 *
 * On save, we composite the current page (image + annotations) back into a
 * single raster blob via `editor.toImage`, so the result can be uploaded as
 * a normal new version of the file and previewed everywhere afterwards.
 */
export const ImageAnnotator = forwardRef<ImageAnnotatorRef, Props>(
  function ImageAnnotator({ imageFile }, ref) {
    const editorRef = useRef<Editor | null>(null);

    useImperativeHandle(
      ref,
      () => ({
        async getOutputBlob(format) {
          const editor = editorRef.current;
          if (!editor) return null;
          const ids = Array.from(editor.getCurrentPageShapeIds());
          if (ids.length === 0) return null;
          const { blob } = await editor.toImage(ids, {
            format,
            background: format !== "png", // JPEG/WEBP have no alpha — paint a white bg
            padding: 0,
            scale: 1,
          });
          return blob;
        },
      }),
      [],
    );

    return (
      <div className="absolute inset-0">
        <Tldraw
          onMount={(editor) => {
            editorRef.current = editor;
            // Insert the image via the same path drag-drop uses. tldraw creates
            // an Image asset + an image shape sized to the natural dimensions.
            editor.putExternalContent({
              type: "files",
              files: [imageFile],
              point: { x: 0, y: 0 },
              ignoreParent: true,
            }).then(() => {
              // Centre and fit the image so the user starts framed on the content.
              editor.zoomToFit({ animation: { duration: 0 } });
            }).catch((err) => {
              // Non-fatal: the canvas is usable, just not pre-loaded.
              console.error("Failed to load image into annotator", err);
            });
          }}
        />
      </div>
    );
  },
);
