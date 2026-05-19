"use client";

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Trash2, Pencil, X, Check } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Skeleton } from "@/components/ui/skeleton";
import { comments as commentsApi, type Comment } from "@/lib/api";
import { formatRelative, parseServerDate } from "@/lib/format";

function wasEdited(c: Comment): boolean {
  const created = parseServerDate(c.created_at).getTime();
  const updated = parseServerDate(c.updated_at).getTime();
  // Same-second creation+update is normal (both default to NOW() on insert).
  // Treat anything >1s apart as an edit.
  return !isNaN(created) && !isNaN(updated) && updated - created > 1000;
}

interface Props {
  fileId: string;
  /**
   * Override the root container's layout classes. The default styles the panel
   * as a right-side aside in PreviewModal (border-l, fixed width). When the
   * panel is hosted in a Dialog instead, the caller passes a class that drops
   * the border / width and constrains height.
   */
  className?: string;
  /**
   * Whether to render the panel's own "Comments" header. The Dialog host
   * supplies its own DialogHeader so it sets this to false to avoid a double
   * heading.
   */
  showHeader?: boolean;
}

const DEFAULT_CLASS = "flex flex-col border-l bg-background w-80 shrink-0";

export function CommentsPanel({ fileId, className = DEFAULT_CLASS, showHeader = true }: Props) {
  const { data, isLoading, mutate } = useSWR(["comments", fileId], () => commentsApi.list(fileId));
  const [body, setBody] = useState("");
  const [posting, setPosting] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editBody, setEditBody] = useState("");

  const post = async () => {
    const text = body.trim();
    if (!text) return;
    setPosting(true);
    try {
      await commentsApi.create(fileId, text);
      setBody("");
      mutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to post comment");
    } finally {
      setPosting(false);
    }
  };

  const startEdit = (c: Comment) => {
    setEditingId(c.id);
    setEditBody(c.body);
  };

  const saveEdit = async () => {
    if (!editingId) return;
    const text = editBody.trim();
    if (!text) return;
    try {
      await commentsApi.update(editingId, text);
      setEditingId(null);
      mutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to edit comment");
    }
  };

  const remove = async (id: string) => {
    try {
      await commentsApi.delete(id);
      mutate();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to delete comment");
    }
  };

  return (
    <aside className={className}>
      {showHeader && (
        <div className="px-4 py-3 border-b shrink-0">
          <h2 className="text-sm font-semibold">Comments</h2>
        </div>
      )}

      <div className="flex-1 min-h-0 overflow-auto px-4 py-3 space-y-3">
        {isLoading && (
          <>
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="flex gap-2">
                <Skeleton className="h-7 w-7 rounded-full shrink-0" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3 w-24" />
                  <Skeleton className="h-3 w-full" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
            ))}
          </>
        )}
        {!isLoading && !data?.comments.length && (
          <p className="text-xs text-muted-foreground">No comments yet.</p>
        )}
        {data?.comments.map((c) => (
          <div key={c.id} className="flex gap-2">
            <Avatar className="h-7 w-7 shrink-0">
              <AvatarFallback className="text-xs">
                {c.username.slice(0, 2).toUpperCase()}
              </AvatarFallback>
            </Avatar>
            <div className="flex-1 min-w-0">
              <div className="flex items-baseline gap-2 mb-0.5">
                <span className="text-xs font-medium truncate">{c.username}</span>
                <span className="text-[10px] text-muted-foreground shrink-0">
                  {formatRelative(c.created_at)}
                </span>
                {wasEdited(c) && (
                  <span
                    className="text-[10px] text-muted-foreground/70 shrink-0 italic"
                    title={`Edited ${formatRelative(c.updated_at)}`}
                  >
                    (edited {formatRelative(c.updated_at)})
                  </span>
                )}
              </div>
              {editingId === c.id ? (
                <div className="space-y-1">
                  <Textarea
                    value={editBody}
                    onChange={(e) => setEditBody(e.target.value)}
                    rows={3}
                    className="text-xs"
                  />
                  <div className="flex gap-1">
                    <Button size="sm" variant="outline" className="h-6 px-2"
                      onClick={() => setEditingId(null)}>
                      <X className="h-3 w-3" />
                    </Button>
                    <Button size="sm" className="h-6 px-2" onClick={saveEdit}>
                      <Check className="h-3 w-3" />
                    </Button>
                  </div>
                </div>
              ) : (
                <>
                  <p className="text-xs whitespace-pre-wrap break-words">{c.body}</p>
                  {c.editable && (
                    <div className="flex gap-1 mt-1">
                      <button onClick={() => startEdit(c)} className="text-[10px] text-muted-foreground hover:text-foreground">
                        <Pencil className="h-3 w-3 inline" /> Edit
                      </button>
                      <button onClick={() => remove(c.id)} className="text-[10px] text-muted-foreground hover:text-destructive ml-1">
                        <Trash2 className="h-3 w-3 inline" /> Delete
                      </button>
                    </div>
                  )}
                </>
              )}
            </div>
          </div>
        ))}
      </div>

      <div className="border-t shrink-0 p-3 space-y-2">
        <Textarea
          placeholder="Write a comment…"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={2}
          className="text-xs"
        />
        <Button size="sm" className="w-full" onClick={post} disabled={posting || !body.trim()}>
          {posting ? "Posting…" : "Post"}
        </Button>
      </div>
    </aside>
  );
}
