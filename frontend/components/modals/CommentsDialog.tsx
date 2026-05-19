"use client";

import { MessageSquare } from "lucide-react";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
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
 */
export function CommentsDialog({ open, onOpenChange, fileId, fileName }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-0 gap-0 overflow-hidden">
        <DialogHeader className="px-4 py-3 border-b">
          <DialogTitle className="flex items-center gap-2">
            <MessageSquare className="h-4 w-4 shrink-0" />
            Comments
          </DialogTitle>
          {fileName && (
            <DialogDescription className="truncate font-mono text-xs" title={fileName}>
              {fileName}
            </DialogDescription>
          )}
        </DialogHeader>

        <CommentsPanel
          fileId={fileId}
          showHeader={false}
          className="flex flex-col bg-background min-h-[400px] max-h-[70vh]"
        />
      </DialogContent>
    </Dialog>
  );
}
