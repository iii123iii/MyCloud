"use client";

// Inline file rename component.
//
// Renders the file/folder name as a button by default. On double-click
// (or F2 with the row focused), it swaps in a controlled <input>. Enter
// commits via the existing rename API; Escape cancels; blur commits.
//
// The component is intentionally self-contained — parents wire it in as a
// drop-in replacement for the plain <span>{name}</span> they had before.
// Optimistic updates are the parent's responsibility (the onRename callback
// is awaited and we revert on throw).

import { useEffect, useId, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { register as registerHotkey } from "@/lib/hotkeys";
import { cn } from "@/lib/utils";

type Props = {
  name: string;
  // onRename receives the new name. Throw to keep the editor open with the
  // old value restored.
  onRename: (newName: string) => Promise<void>;
  // disabled hides the rename affordance entirely (e.g. for read-only shares).
  disabled?: boolean;
  className?: string;
};

export function InlineRename({ name, onRename, disabled, className }: Props) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const errorId = useId();
  const hintId = useId();

  // Whenever the parent updates the canonical name (e.g. after a successful
  // SWR refresh), pick up the new value while the user isn't actively
  // editing.
  useEffect(() => {
    if (!editing) setDraft(name);
  }, [name, editing]);

  // F2 → enter edit mode when the host element is focused.
  useEffect(() => {
    if (disabled) return;
    return registerHotkey("f2", () => {
      setEditing(true);
    });
  }, [disabled]);

  useEffect(() => {
    if (editing) {
      // Move caret to before the extension so users can rename without
      // accidentally clobbering ".pdf".
      const dot = draft.lastIndexOf(".");
      const sel = dot > 0 ? dot : draft.length;
      requestAnimationFrame(() => {
        const el = inputRef.current;
        if (el) {
          el.focus();
          el.setSelectionRange(0, sel);
        }
      });
    } else {
      // Clear any stale error when exiting edit mode.
      setError(null);
    }
    // We intentionally exclude `draft` so re-entering edit mode doesn't keep
    // re-selecting as the user types.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editing]);

  const commit = async () => {
    const trimmed = draft.trim();
    if (trimmed === "" || trimmed === name) {
      setEditing(false);
      setDraft(name);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onRename(trimmed);
      setEditing(false);
    } catch (e) {
      // Revert to old name and surface an inline error. Toasts are still the
      // caller's responsibility — this is just a localized cue.
      setDraft(name);
      setError(e instanceof Error && e.message ? e.message : "Rename failed");
    } finally {
      setBusy(false);
    }
  };

  if (disabled || !editing) {
    return (
      // `block` + `truncate` + `max-w-full min-w-0` constrains the name to
      // its parent's width and shows an ellipsis on overflow. Without
      // `block`, an inline <span> ignores width constraints entirely and
      // the card grows past its grid cell on very long names.
      <span
        className={cn(
          "block truncate max-w-full min-w-0 rounded-sm",
          "transition-colors",
          !disabled &&
            "cursor-text hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          className,
        )}
        // Double-click activates edit mode. We don't bind onClick because
        // single-click usually means "open" in a file explorer.
        onDoubleClick={() => !disabled && setEditing(true)}
        tabIndex={0}
        title={name}
      >
        {name}
      </span>
    );
  }

  return (
    <span
      className={cn(
        "relative inline-flex items-center w-full max-w-[12rem] min-w-0",
        "transition-transform duration-150 ease-out scale-[1.01]",
      )}
    >
      <input
        ref={inputRef}
        // Opt the input out of global hotkeys so F2 doesn't toggle while typing.
        data-no-hotkeys="true"
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          if (error) setError(null);
        }}
        disabled={busy}
        aria-label="Rename"
        aria-busy={busy || undefined}
        aria-invalid={error ? true : undefined}
        aria-describedby={cn(error ? errorId : null, hintId)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            void commit();
          } else if (e.key === "Escape") {
            e.preventDefault();
            setDraft(name);
            setError(null);
            setEditing(false);
          }
        }}
        onBlur={() => void commit()}
        className={cn(
          // Match the surrounding label footprint: block-level, full width
          // with the same max as the static span, same text size, same
          // truncation behavior on overflow.
          "block w-full min-w-0 truncate",
          "bg-background rounded border px-1 text-sm leading-[inherit] font-[inherit]",
          "border-border shadow-none outline-none",
          "transition-[border-color,box-shadow,background-color] duration-150",
          "hover:border-ring/60",
          "focus-visible:border-ring focus-visible:shadow-[0_0_0_3px_var(--ring,theme(colors.ring))/30] focus-visible:ring-2 focus-visible:ring-ring/40",
          "disabled:cursor-not-allowed disabled:opacity-60",
          error &&
            "border-destructive focus-visible:border-destructive focus-visible:ring-destructive/40",
          // Pad the right side when a spinner is shown so the caret never
          // collides with it.
          busy && "pr-6",
          className,
        )}
      />
      {busy && (
        <Loader2
          className="pointer-events-none absolute right-1.5 size-3.5 animate-spin text-muted-foreground"
          aria-hidden="true"
        />
      )}
      {/* Visually hidden hint always present, so screen readers describe the
          shortcuts without re-announcing on every keystroke. */}
      <span id={hintId} className="sr-only">
        Press Enter to save, Escape to cancel.
      </span>
      {error && (
        <span
          id={errorId}
          role="alert"
          className="absolute left-0 top-full mt-0.5 truncate max-w-full text-xs text-destructive"
        >
          {error}
        </span>
      )}
    </span>
  );
}
