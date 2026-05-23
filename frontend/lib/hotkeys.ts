// Global hotkey infrastructure.
//
// Lightweight wrapper around `document.addEventListener('keydown')` —
// avoids the react-hotkeys-hook dep for a small set of needs. Components
// register chords (e.g. "mod+k", "f2") and the matching keydown fires the
// callback. Inputs and editors opt out via `data-no-hotkeys` on any
// ancestor.
//
// Chord syntax:
//   "mod"   = Ctrl on Windows/Linux, Cmd on macOS
//   "meta"  = explicit Cmd
//   "ctrl"  = explicit Ctrl
//   "shift", "alt", "option"
//   key tokens are matched against KeyboardEvent.key (case-insensitive)
//   examples: "mod+k", "shift+?", "f2", "esc", "ctrl+shift+p"

type Handler = (e: KeyboardEvent) => void;

type Registration = {
  chord: string;
  fn: Handler;
};

const registry = new Set<Registration>();
let listenerInstalled = false;

function install() {
  if (listenerInstalled || typeof window === "undefined") return;
  listenerInstalled = true;
  // capture: true so we run BEFORE the browser's built-in handlers for
  // chords like Cmd+K (Firefox focuses search bar, Chrome focuses URL).
  // Our handlers e.preventDefault() to keep those from interfering.
  window.addEventListener(
    "keydown",
    (e) => {
      // Skip when typing in inputs, textareas, contentEditable elements, or
      // anything within an opt-out subtree. Otherwise typing a 'k' into the
      // search box would open the command palette.
      if (shouldSkip(e.target)) return;
      for (const reg of registry) {
        if (matches(reg.chord, e)) {
          reg.fn(e);
        }
      }
    },
    { capture: true },
  );
}

function shouldSkip(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return true;
  if (target.isContentEditable) return true;
  if (target.closest("[data-no-hotkeys]")) return true;
  return false;
}

function isMac(): boolean {
  if (typeof navigator === "undefined") return false;
  return /mac|iphone|ipad|ipod/i.test(navigator.platform);
}

// matches resolves a chord like "mod+k" against an actual KeyboardEvent.
export function matches(chord: string, e: KeyboardEvent): boolean {
  const parts = chord
    .toLowerCase()
    .split("+")
    .map((p) => p.trim())
    .filter(Boolean);
  let wantMeta = false;
  let wantCtrl = false;
  let wantShift = false;
  let wantAlt = false;
  let wantKey = "";
  for (const part of parts) {
    switch (part) {
      case "mod":
        if (isMac()) wantMeta = true; else wantCtrl = true; break;
      case "meta":
      case "cmd":
      case "command":
        wantMeta = true; break;
      case "ctrl":
      case "control":
        wantCtrl = true; break;
      case "shift":
        wantShift = true; break;
      case "alt":
      case "option":
        wantAlt = true; break;
      case "esc":
        wantKey = "escape"; break;
      default:
        wantKey = part;
    }
  }
  if (wantMeta !== e.metaKey) return false;
  if (wantCtrl !== e.ctrlKey) return false;
  if (wantShift !== e.shiftKey) return false;
  if (wantAlt !== e.altKey) return false;
  return wantKey === "" || e.key.toLowerCase() === wantKey;
}

// register installs a chord+handler. Returns an unregister fn for cleanup.
// Components MUST call the unregister in useEffect cleanup to avoid leaks.
export function register(chord: string, fn: Handler): () => void {
  install();
  const reg: Registration = { chord, fn };
  registry.add(reg);
  return () => {
    registry.delete(reg);
  };
}
