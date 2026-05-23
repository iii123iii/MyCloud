"use client";

import { MessageSquare } from "lucide-react";

import { cn } from "@/lib/utils";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { CommentsPanel } from "./CommentsPanel";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileId: string;
  fileName?: string;
}

/**
 * Standalone Comments view — lets the user read/post comments on a file
 * without paying for a full PreviewModal (which would fetch the file blob and
 * mount mammoth / SheetJS / Shiki for nothing if the user just wants to chat).
 *
 * Wraps the same CommentsPanel that PreviewModal uses, so writes mounted in
 * either surface revalidate the other via the shared SWR key.
 *
 * Overflow strategy: DialogContent is a constrained flex column (capped at
 * 90vh / viewport width minus gutters on mobile). The header is pinned and
 * truncates long filenames behind a Tooltip. The hosted CommentsPanel handles
 * its own internal scrolling for the comments list while keeping its composer
 * pinned, with the panel-as-a-whole capped at 60vh so the list never pushes
 * the composer off-screen or makes the dialog overflow the viewport. We
 * intentionally avoid `overflow-hidden` on the popup itself so that any
 * per-comment dropdown menus portaled by shadcn primitives are free to
 * collision-detect outside the dialog's clip box.
 */
export function CommentsDialog({ open, onOpenChange, fileId, fileName }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          // Layout: flex column so header stays pinned and the panel below
          // can flex-fill and own its internal scroll region.
          "flex flex-col p-0 gap-0",
          // Width: keep narrow viewports usable — leave a 1rem gutter on each
          // side so the dialog (and its composer row / send button) can't
          // induce horizontal scroll on mobile.
          "w-[calc(100vw-2rem)] sm:w-full sm:max-w-md",
          // Height: cap the whole dialog so it never blows past the viewport;
          // the internal list scrolls instead of the dialog growing.
          "max-h-[90vh]",
        )}
      >
        <DialogHeader className="px-4 py-3 border-b shrink-0 min-w-0">
          <DialogTitle className="flex items-center gap-2 min-w-0">
            <MessageSquare className="h-4 w-4 shrink-0" />
            <span className="truncate">Comments</span>
          </DialogTitle>
          {fileName && (
            <TooltipProvider delay={300}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <DialogDescription
                      className="truncate font-mono text-xs max-w-full cursor-default"
                      aria-label={fileName}
                    >
                      {fileName}
                    </DialogDescription>
                  }
                />
                <TooltipContent
                  side="bottom"
                  align="start"
                  sideOffset={6}
                  className="max-w-[min(90vw,32rem)] break-all font-mono"
                >
                  {fileName}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
        </DialogHeader>

        <CommentsPanel
          fileId={fileId}
          showHeader={false}
          // flex-1 lets the panel claim the remaining dialog height so its
          // internal `flex-1 min-h-0 overflow-auto` list scrolls (the
          // composer stays pinned to the bottom of the panel). Capping the
          // panel at 60vh mirrors the spec for the scrollable list region
          // while leaving room for the dialog header and the panel's
          // composer; min-h-[400px] floors the panel so very short viewports
          // don't collapse the list to zero. overflow-hidden clips the panel
          // edge but the panel's children (notably menus/tooltips spawned by
          // CommentsPanel) are portaled out of this subtree, so per-comment
          // dropdowns can still collision-detect outside the dialog.
          className="flex flex-col flex-1 bg-background min-h-[400px] max-h-[60vh] overflow-hidden"
        />
      </DialogContent>
    </Dialog>
  );
}
