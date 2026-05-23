// Column visibility + ordering preferences.
//
// Stored per-user in localStorage so the UI remembers "I always want size
// in the third column, hide updated_at" between sessions. The data model
// is intentionally tiny so it can flow through SSR (Next renders the
// initial state from the default; the hook hydrates from localStorage on
// mount).

import { useEffect, useMemo, useState } from "react";

export type ColumnKey = "name" | "size_bytes" | "mime_type" | "created_at" | "updated_at";

export type ColumnSpec = {
  key: ColumnKey;
  label: string;
  // sortable rows in the list view click the header to toggle ASC/DESC;
  // unsortable columns just stay where they are.
  sortable: boolean;
};

// The canonical list. Order here is the default visible order.
export const DEFAULT_COLUMNS: ColumnSpec[] = [
  { key: "name",       label: "Name",     sortable: true  },
  { key: "size_bytes", label: "Size",     sortable: true  },
  { key: "mime_type",  label: "Type",     sortable: false },
  { key: "updated_at", label: "Modified", sortable: true  },
  { key: "created_at", label: "Created",  sortable: true  },
];

type StoredPrefs = {
  order: ColumnKey[];
  hidden: ColumnKey[];
};

const STORAGE_KEY = "mc_column_prefs";

function readStored(): StoredPrefs | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const obj = JSON.parse(raw) as StoredPrefs;
    if (!Array.isArray(obj.order) || !Array.isArray(obj.hidden)) return null;
    return obj;
  } catch {
    return null;
  }
}

function writeStored(p: StoredPrefs) {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(p));
  } catch {
    // QuotaExceeded is the only realistic failure; swallow.
  }
}

// useColumnPrefs returns the visible columns in their persisted order plus
// setters that update both state and localStorage atomically.
export function useColumnPrefs() {
  const [order, setOrder] = useState<ColumnKey[]>(DEFAULT_COLUMNS.map((c) => c.key));
  const [hidden, setHidden] = useState<Set<ColumnKey>>(new Set());

  // Hydrate from localStorage on mount.
  useEffect(() => {
    const stored = readStored();
    if (stored) {
      // Ensure any newly-added columns appear by appending those missing
      // from the stored order — keeps upgrades graceful.
      const known = new Set(DEFAULT_COLUMNS.map((c) => c.key));
      const seen = new Set<ColumnKey>();
      const merged: ColumnKey[] = [];
      for (const k of stored.order) {
        if (known.has(k) && !seen.has(k)) {
          merged.push(k);
          seen.add(k);
        }
      }
      for (const c of DEFAULT_COLUMNS) {
        if (!seen.has(c.key)) merged.push(c.key);
      }
      // Hydrate persisted column prefs after mount. Reading localStorage
      // synchronously during useState init would break SSR.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setOrder(merged);
      setHidden(new Set(stored.hidden.filter((k) => known.has(k))));
    }
  }, []);

  const persist = (nextOrder: ColumnKey[], nextHidden: Set<ColumnKey>) => {
    writeStored({ order: nextOrder, hidden: Array.from(nextHidden) });
  };

  const toggleVisible = (key: ColumnKey) => {
    setHidden((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      persist(order, next);
      return next;
    });
  };

  const moveTo = (key: ColumnKey, index: number) => {
    setOrder((prev) => {
      const next = prev.filter((k) => k !== key);
      next.splice(Math.max(0, Math.min(index, next.length)), 0, key);
      persist(next, hidden);
      return next;
    });
  };

  const reset = () => {
    const defaults = DEFAULT_COLUMNS.map((c) => c.key);
    setOrder(defaults);
    setHidden(new Set());
    persist(defaults, new Set());
  };

  const visibleColumns = useMemo(() => {
    const byKey = new Map(DEFAULT_COLUMNS.map((c) => [c.key, c]));
    return order
      .filter((k) => !hidden.has(k))
      .map((k) => byKey.get(k))
      .filter((c): c is ColumnSpec => c !== undefined);
  }, [order, hidden]);

  return {
    visibleColumns,
    isHidden: (k: ColumnKey) => hidden.has(k),
    toggleVisible,
    moveTo,
    reset,
  };
}
