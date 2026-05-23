"use client";

// Marquee (drag-rectangle) selection — the translucent blue box you drag
// across a Finder / Windows Explorer window to select multiple items.
//
// Usage from the grid: render <MarqueeOverlay containerRef={gridRef}
// onSelectionChange={(ids, additive) => ...} /> inside the same wrapper as
// the grid. The component listens for mousedown on the container, tracks
// pointer movement, hit-tests every element that carries a `data-file-id`
// or `data-folder-id` attribute, and reports the matching ids.
//
// `additive` is true when Ctrl/Cmd/Shift is held — the grid adds these
// ids to its existing selection rather than replacing it.
//
// We bail out (no marquee) when:
//   - the mousedown target is an interactive element (card, button, link)
//   - the mousedown is on the right button
//   - the user only moves <4 px (so a regular click on empty space clears
//     selection rather than starting an accidental marquee)

import { useEffect, useRef, useState } from "react";

type Props = {
  containerRef: React.RefObject<HTMLElement | null>;
  // Called whenever the marquee selection changes. ids is the set of items
  // currently inside the rectangle. additive=true when the user held a
  // modifier; the caller should add, not replace.
  onSelectionChange: (ids: string[], additive: boolean) => void;
  // Called on a clean click (mousedown→up with no drag) on the empty
  // canvas. Caller usually clears its selection here.
  onClickEmpty?: () => void;
};

type Rect = { x: number; y: number; w: number; h: number };

const DRAG_THRESHOLD_PX = 4;

export function MarqueeOverlay({ containerRef, onSelectionChange, onClickEmpty }: Props) {
  // Rectangle in container-relative coordinates (so we can render an
  // absolutely-positioned div inside the same parent).
  const [rect, setRect] = useState<Rect | null>(null);

  // Refs hold values that the global mousemove/mouseup handlers need to
  // read without re-attaching listeners on every state change.
  const startRef = useRef<{ x: number; y: number; additive: boolean } | null>(null);
  const draggingRef = useRef(false);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const onMouseDown = (e: MouseEvent) => {
      // Only the primary button kicks off a marquee — right-click opens the
      // context menu, middle-click is browser-defined.
      if (e.button !== 0) return;
      // Skip when the mousedown landed on an interactive child. We treat
      // anything carrying data-file-id / data-folder-id / a button / a
      // link as an item the user actually wants to click.
      const target = e.target as HTMLElement | null;
      if (!target) return;
      if (target.closest("[data-file-id], [data-folder-id], button, a, input, textarea, [role='button']")) {
        return;
      }
      const bounds = container.getBoundingClientRect();
      startRef.current = {
        x: e.clientX - bounds.left,
        y: e.clientY - bounds.top,
        additive: e.shiftKey || e.ctrlKey || e.metaKey,
      };
      draggingRef.current = false;
    };

    const onMouseMove = (e: MouseEvent) => {
      const start = startRef.current;
      if (!start) return;
      const bounds = container.getBoundingClientRect();
      const cx = e.clientX - bounds.left;
      const cy = e.clientY - bounds.top;
      const dx = cx - start.x;
      const dy = cy - start.y;
      // Wait until the user has moved enough to call it a drag — otherwise
      // a plain click would also paint a 1-px marquee and clear selection.
      if (!draggingRef.current && Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) {
        return;
      }
      draggingRef.current = true;
      const newRect: Rect = {
        x: Math.min(start.x, cx),
        y: Math.min(start.y, cy),
        w: Math.abs(dx),
        h: Math.abs(dy),
      };
      setRect(newRect);
      // Hit-test every grid item against the marquee rectangle. Container-
      // relative coords match the rectangle's coords, so we just query the
      // items and compute their offset within the container.
      const ids = collectIdsInRect(container, newRect);
      onSelectionChange(ids, start.additive);
      // Prevent text selection across the page while dragging.
      e.preventDefault();
    };

    const onMouseUp = () => {
      const wasDragging = draggingRef.current;
      const hadStart = startRef.current !== null;
      startRef.current = null;
      draggingRef.current = false;
      setRect(null);
      // If the user clicked without dragging on empty space, clear the
      // selection (matches OS Explorer behaviour).
      if (hadStart && !wasDragging && onClickEmpty) {
        onClickEmpty();
      }
    };

    container.addEventListener("mousedown", onMouseDown);
    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      container.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, [containerRef, onSelectionChange, onClickEmpty]);

  if (!rect) return null;

  return (
    <div
      // Pointer-events: none so the marquee overlay doesn't swallow the
      // mousemove events we need on the underlying grid.
      className="pointer-events-none absolute z-10 border border-primary/70 bg-primary/15 rounded-sm"
      style={{
        left: rect.x,
        top: rect.y,
        width: rect.w,
        height: rect.h,
      }}
      aria-hidden="true"
    />
  );
}

// collectIdsInRect walks the container's [data-file-id] and [data-folder-id]
// descendants and returns the ids whose bounding box intersects the marquee
// rectangle. /files:batch accepts both file_ids and folder_ids, so a
// marquee that crosses both types now reports a mixed selection; the
// SelectionToolbar partitions the ids back into the two arrays at post time.
function collectIdsInRect(container: HTMLElement, rect: Rect): string[] {
  const bounds = container.getBoundingClientRect();
  const items = container.querySelectorAll<HTMLElement>(
    "[data-file-id], [data-folder-id]",
  );
  const out: string[] = [];
  for (const el of items) {
    const r = el.getBoundingClientRect();
    const ex = r.left - bounds.left;
    const ey = r.top - bounds.top;
    const ew = r.width;
    const eh = r.height;
    // AABB intersection test.
    if (
      ex < rect.x + rect.w &&
      ex + ew > rect.x &&
      ey < rect.y + rect.h &&
      ey + eh > rect.y
    ) {
      const id =
        el.getAttribute("data-file-id") ?? el.getAttribute("data-folder-id");
      if (id) out.push(id);
    }
  }
  return out;
}
