/**
 * Folder-aware drag-and-drop traversal helpers.
 *
 * react-dropzone strips folder structure by default — only File entries make
 * it through. These helpers walk DataTransferItem entries recursively via
 * the legacy `webkitGetAsEntry` API (supported in all evergreen browsers),
 * emitting each contained File together with its relative path under the
 * dropped root.
 */

export interface DroppedFile {
  file: File;
  /** Slash-separated path relative to the drop root, e.g. "docs/2024/notes.md". */
  relativePath: string;
}

interface FsEntry {
  isFile: boolean;
  isDirectory: boolean;
  name: string;
  fullPath: string;
  file?: (callback: (file: File) => void, err?: (err: unknown) => void) => void;
  createReader?: () => FsDirectoryReader;
}

interface FsDirectoryReader {
  readEntries: (
    callback: (entries: FsEntry[]) => void,
    err?: (err: unknown) => void,
  ) => void;
}

function readEntriesAll(reader: FsDirectoryReader): Promise<FsEntry[]> {
  return new Promise<FsEntry[]>((resolve, reject) => {
    const acc: FsEntry[] = [];
    const step = () => {
      reader.readEntries((entries) => {
        if (!entries.length) {
          resolve(acc);
          return;
        }
        acc.push(...entries);
        step();
      }, reject);
    };
    step();
  });
}

async function walkEntry(entry: FsEntry, base: string): Promise<DroppedFile[]> {
  if (entry.isFile && entry.file) {
    const file = await new Promise<File>((res, rej) => entry.file!(res, rej));
    return [{ file, relativePath: base ? `${base}/${entry.name}` : entry.name }];
  }
  if (entry.isDirectory && entry.createReader) {
    const reader = entry.createReader();
    const children = await readEntriesAll(reader);
    const out: DroppedFile[] = [];
    for (const c of children) {
      out.push(...(await walkEntry(c, base ? `${base}/${entry.name}` : entry.name)));
    }
    return out;
  }
  return [];
}

/**
 * Walk a DataTransfer's items list, recursively expanding any dropped folders.
 * Returns a flat list of files paired with their relative paths.
 *
 * If the browser does not support webkitGetAsEntry, falls back to dataTransfer.files.
 */
export async function collectDroppedFiles(
  dataTransfer: DataTransfer,
): Promise<DroppedFile[]> {
  if (!dataTransfer.items || !dataTransfer.items.length) {
    return Array.from(dataTransfer.files).map((file) => ({ file, relativePath: file.name }));
  }
  const entries: FsEntry[] = [];
  for (const item of Array.from(dataTransfer.items)) {
    const anyItem = item as unknown as {
      webkitGetAsEntry?: () => FsEntry | null;
    };
    const entry = anyItem.webkitGetAsEntry?.();
    if (entry) entries.push(entry);
  }
  if (!entries.length) {
    return Array.from(dataTransfer.files).map((file) => ({ file, relativePath: file.name }));
  }
  const out: DroppedFile[] = [];
  for (const entry of entries) {
    out.push(...(await walkEntry(entry, "")));
  }
  return out;
}

/**
 * Given a list of dropped files (with relative paths under the drop root),
 * group them by parent directory and return:
 *   directories: sorted breadth-first so parents are created before children
 *   filesByDir : map of directory path → DroppedFile[]
 */
export function planDroppedTree(items: DroppedFile[]) {
  const dirs = new Set<string>();
  const filesByDir = new Map<string, DroppedFile[]>();
  for (const item of items) {
    const dir = item.relativePath.includes("/")
      ? item.relativePath.slice(0, item.relativePath.lastIndexOf("/"))
      : "";
    if (dir) {
      let cur = "";
      for (const seg of dir.split("/")) {
        cur = cur ? `${cur}/${seg}` : seg;
        dirs.add(cur);
      }
    }
    const bucket = filesByDir.get(dir) ?? [];
    bucket.push(item);
    filesByDir.set(dir, bucket);
  }
  const directories = Array.from(dirs).sort((a, b) => a.split("/").length - b.split("/").length);
  return { directories, filesByDir };
}
