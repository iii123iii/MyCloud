import { useState } from "react";
import {
  FileText, FileImage, FileVideo, FileAudio, FileArchive,
  FileSpreadsheet, FileCode, Folder, File,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Skeleton } from "@/components/ui/skeleton";

interface Props {
  mime?: string;
  isFolder?: boolean;
  className?: string;
  // Optional thumbnail URL — when provided, the component renders an <img>
  // with a Skeleton placeholder, fades in on load, and falls back to the
  // type icon if loading fails.
  thumbnailUrl?: string;
  // File name used to derive descriptive alt text and the fallback letter
  // tile. When omitted, alt text falls back to the type label.
  fileName?: string;
  // Size variant keeps padding/box ratios consistent across call sites.
  // When omitted the component renders only the bare lucide icon and any
  // passed-in className is applied directly to it — this matches the
  // historical behavior so existing call sites are not affected.
  size?: "sm" | "md" | "lg" | "xl";
  // When true the wrapper becomes a focusable button-like element with a
  // visible focus-visible ring and hover/active transitions.
  interactive?: boolean;
  onClick?: () => void;
}

// Maps a mime type to a lucide icon component plus a textual type label
// (used for accessible alt/aria-label so the color-coded badge is not the
// only signal of file type).
function pickType(mime: string, isFolder?: boolean) {
  if (isFolder)                       return { Icon: Folder,          color: "text-yellow-500",       label: "Folder"      };
  if (mime.startsWith("image/"))      return { Icon: FileImage,       color: "text-emerald-500",      label: "Image"       };
  if (mime.startsWith("video/"))      return { Icon: FileVideo,       color: "text-violet-500",       label: "Video"       };
  if (mime.startsWith("audio/"))      return { Icon: FileAudio,       color: "text-pink-500",         label: "Audio"       };
  if (mime === "application/pdf")     return { Icon: FileText,        color: "text-red-500",          label: "PDF"         };
  if (mime.startsWith("text/"))       return { Icon: FileCode,        color: "text-sky-500",          label: "Text"        };
  if (mime.includes("zip") || mime.includes("compressed") || mime.includes("tar"))
    return { Icon: FileArchive,    color: "text-amber-500",        label: "Archive"     };
  if (mime.includes("excel") || mime.includes("spreadsheet"))
    return { Icon: FileSpreadsheet, color: "text-green-600",       label: "Spreadsheet" };
  return                                  { Icon: File,             color: "text-muted-foreground", label: "File"        };
}

// Per-variant sizing keeps the icon-to-box ratio visually consistent. The
// icon itself sits at ~60% of the box edge so padding feels even at every
// size. Used only when a `size` prop is supplied.
const BOX_SIZES = {
  sm: "h-8 w-8  rounded-md",
  md: "h-10 w-10 rounded-md",
  lg: "h-14 w-14 rounded-lg",
  xl: "h-20 w-20 rounded-xl",
} as const;

const ICON_SIZES = {
  sm: "h-5 w-5",
  md: "h-6 w-6",
  lg: "h-8 w-8",
  xl: "h-12 w-12",
} as const;

export function FileIcon({
  mime = "",
  isFolder,
  className,
  thumbnailUrl,
  fileName,
  size,
  interactive,
  onClick,
}: Props) {
  const { Icon, color, label } = pickType(mime, isFolder);

  // Loading/error state for the optional thumbnail. `loaded` drives the
  // fade-in; `failed` causes a graceful fallback to the type icon.
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);

  // Legacy / bare-icon path — preserves the original DOM exactly so every
  // existing call site (which passes only mime/isFolder/className) renders
  // the same single <svg> as before, no wrapper, no behavior change.
  if (!size && !thumbnailUrl && !interactive) {
    return (
      <Icon
        aria-hidden="true"
        className={cn(color, className)}
      />
    );
  }

  // Boxed variant — used when any of `size`, `thumbnailUrl`, or
  // `interactive` is supplied. The wrapper centers the icon, hosts the
  // thumbnail layer, and carries hover/focus styling.
  const boxSize = size ? BOX_SIZES[size] : BOX_SIZES.md;
  const iconSize = size ? ICON_SIZES[size] : ICON_SIZES.md;

  const showThumb = !!thumbnailUrl && !failed;
  const altText = fileName
    ? `${fileName} (${label.toLowerCase()})`
    : `${label} file`;

  const Wrapper: "button" | "div" = interactive ? "button" : "div";

  return (
    <Wrapper
      type={interactive ? "button" : undefined}
      onClick={interactive ? onClick : undefined}
      aria-label={interactive ? altText : undefined}
      className={cn(
        "relative inline-flex shrink-0 items-center justify-center overflow-hidden bg-muted/40",
        boxSize,
        interactive && [
          "cursor-pointer outline-none transition-all duration-200 ease-out",
          "hover:bg-muted/70 active:scale-[0.98]",
          "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        ],
        className,
      )}
    >
      {showThumb ? (
        <>
          {/* Loading placeholder — visible until the <img> reports load. */}
          {!loaded && (
            <Skeleton
              aria-hidden="true"
              className="absolute inset-0 h-full w-full rounded-none"
            />
          )}
          <img
            src={thumbnailUrl}
            alt={altText}
            loading="lazy"
            decoding="async"
            draggable={false}
            onLoad={() => setLoaded(true)}
            onError={() => setFailed(true)}
            className={cn(
              "absolute inset-0 h-full w-full object-cover",
              "transition-opacity duration-300 ease-out",
              loaded ? "opacity-100" : "opacity-0",
            )}
          />
          {/* Decorative type tint underneath — only painted while the
              thumbnail is still loading so the box never looks empty. */}
          {!loaded && (
            <Icon
              aria-hidden="true"
              className={cn(iconSize, color, "opacity-40")}
            />
          )}
        </>
      ) : (
        <>
          <Icon
            aria-hidden="true"
            className={cn(iconSize, color)}
          />
          {/* Visually-hidden textual label so the color-coded type is also
              conveyed to assistive tech (non-interactive variant only —
              interactive variant already has aria-label on the wrapper). */}
          {!interactive && (
            <span className="sr-only">{altText}</span>
          )}
        </>
      )}
    </Wrapper>
  );
}
