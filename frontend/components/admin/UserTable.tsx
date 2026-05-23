"use client";

import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { UserPlus, Pencil, Trash2, ShieldCheck, ShieldOff, MoreHorizontal } from "lucide-react";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

import { admin as adminApi, auth as authApi } from "@/lib/api";
import { cn } from "@/lib/utils";
import { formatBytes, formatDate } from "@/lib/format";
import type { User } from "@/lib/types";

interface Props {
  users: User[];
  onMutate: () => void;
}

interface EditState {
  user: User | null;
  newPassword: string;
  quota: string;
  role: string;
}

export function UserTable({ users, onMutate }: Props) {
  const [editState, setEditState] = useState<EditState | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newUser, setNewUser] = useState({ username: "", email: "", password: "", role: "user", quota: "10" });
  // Shares the "me" SWR key with the rest of the app so this is a free
  // lookup, not a fresh round-trip. Used to suppress disable/delete on the
  // current admin's own row.
  const { data: me } = useSWR("me", authApi.me, { shouldRetryOnError: false });

  // Disable & delete are blocked for two reasons: (1) you can't act on
  // yourself — that would lock you out of the panel mid-action; (2) admins
  // can't disable/delete each other through this UI, peer-admin role
  // changes go through the explicit Edit dialog instead.
  const lockReason = (user: User): string | null => {
    if (me && user.id === me.id) return "You can't act on your own account";
    if (user.role === "admin") return "Admin accounts are protected";
    return null;
  };

  const toggleActive = async (user: User) => {
    const reason = lockReason(user);
    if (reason) {
      toast.error(reason);
      return;
    }
    try {
      await adminApi.updateUser(user.id, { is_active: !user.is_active });
      onMutate();
      toast.success(user.is_active ? "User disabled" : "User enabled");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to update user");
    }
  };

  const deleteUser = async (user: User) => {
    const reason = lockReason(user);
    if (reason) {
      toast.error(reason);
      return;
    }
    if (!confirm(`Delete user "${user.username}" and permanently remove all of their files and folders? This cannot be undone.`)) return;
    try {
      await adminApi.deleteUser(user.id);
      onMutate();
      toast.success("User deleted");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to delete user");
    }
  };

  const saveEdit = async () => {
    if (!editState?.user) return;
    try {
      const updates: Partial<User & { password: string }> = {
        role:        editState.role as "admin" | "user",
        quota_bytes: Math.round(parseFloat(editState.quota) * 1_073_741_824),
      };
      if (editState.newPassword) updates.password = editState.newPassword;
      await adminApi.updateUser(editState.user.id, updates);
      onMutate();
      toast.success("User updated");
      setEditState(null);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to update user");
    }
  };

  const createUser = async () => {
    try {
      await adminApi.createUser({
        username: newUser.username,
        email: newUser.email,
        password: newUser.password,
        role: newUser.role as "admin" | "user",
        quota_bytes: Math.round(parseFloat(newUser.quota) * 1_073_741_824),
      } as User & { password: string });
      onMutate();
      toast.success("User created");
      setCreateOpen(false);
      setNewUser({ username: "", email: "", password: "", role: "user", quota: "10" });
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create user");
    }
  };

  // Truncated cell with tooltip showing the full value.
  const TruncCell = ({ value, className }: { value: string; className?: string }) => (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            className={cn(
              "block max-w-full truncate align-middle cursor-default",
              className,
            )}
          >
            {value}
          </span>
        }
      />
      <TooltipContent>{value}</TooltipContent>
    </Tooltip>
  );

  return (
    <TooltipProvider>
      <div className="flex justify-end mb-3">
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <UserPlus className="h-4 w-4 mr-1.5" />
          New user
        </Button>
      </div>

      {/*
        Outer wrapper keeps the rounded border. We intentionally do NOT set
        overflow-hidden here so the per-row dropdown (which portals out anyway)
        can never be clipped by the container. The inner <Table> already
        provides its own overflow-x-auto scroll container with a visible
        scrollbar on small viewports.
      */}
      <div className="border rounded-lg">
        <Table className="table-fixed w-full min-w-[720px]">
          <TableHeader>
            <TableRow>
              <TableHead className="sticky left-0 z-20 bg-background w-[180px] min-w-[140px]">
                Username
              </TableHead>
              <TableHead className="w-[240px] min-w-[180px]">Email</TableHead>
              <TableHead className="w-[96px]">Role</TableHead>
              <TableHead className="w-[180px] min-w-[160px]">Storage</TableHead>
              <TableHead className="w-[100px]">Status</TableHead>
              <TableHead className="w-[120px]">Joined</TableHead>
              <TableHead className="w-12 text-right" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((user) => (
              <TableRow key={user.id}>
                <TableCell className="font-medium sticky left-0 z-10 bg-background max-w-[180px]">
                  <TruncCell value={user.username} className="font-medium" />
                </TableCell>
                <TableCell className="text-muted-foreground text-sm max-w-[240px]">
                  <TruncCell value={user.email} className="text-muted-foreground text-sm" />
                </TableCell>
                <TableCell className="max-w-[96px]">
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span className="inline-block max-w-full align-middle">
                          <Badge
                            variant={user.role === "admin" ? "default" : "secondary"}
                            className="max-w-full truncate"
                          >
                            <span className="truncate">{user.role}</span>
                          </Badge>
                        </span>
                      }
                    />
                    <TooltipContent>{user.role}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                  {formatBytes(user.used_bytes)} / {formatBytes(user.quota_bytes)}
                </TableCell>
                <TableCell>
                  <Badge variant={user.is_active ? "outline" : "destructive"}>
                    {user.is_active ? "Active" : "Disabled"}
                  </Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                  {formatDate(user.created_at)}
                </TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      aria-label={`Actions for ${user.username}`}
                      className={cn(
                        "inline-flex h-7 w-7 items-center justify-center rounded-md",
                        "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                      )}
                    >
                      <MoreHorizontal className="h-4 w-4" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" sideOffset={4} className="min-w-[160px]">
                      <DropdownMenuItem
                        onClick={() =>
                          setEditState({
                            user,
                            newPassword: "",
                            quota: String(user.quota_bytes / 1_073_741_824),
                            role: user.role,
                          })
                        }
                      >
                        <Pencil className="h-3.5 w-3.5" />
                        Edit
                      </DropdownMenuItem>
                      {(() => {
                        const reason = lockReason(user);
                        // Enabling a disabled non-admin/non-self is fine —
                        // only the disable + delete directions are guarded.
                        const disableLocked = user.is_active && !!reason;
                        const deleteLocked = !!reason;
                        return (
                          <>
                            <DropdownMenuItem
                              onClick={() => toggleActive(user)}
                              disabled={disableLocked}
                              title={disableLocked ? reason ?? undefined : undefined}
                            >
                              {user.is_active ? (
                                <>
                                  <ShieldOff className="h-3.5 w-3.5" />
                                  Disable
                                </>
                              ) : (
                                <>
                                  <ShieldCheck className="h-3.5 w-3.5" />
                                  Enable
                                </>
                              )}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              onClick={() => deleteUser(user)}
                              disabled={deleteLocked}
                              title={deleteLocked ? reason ?? undefined : undefined}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                              Delete
                            </DropdownMenuItem>
                          </>
                        );
                      })()}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Edit dialog */}
      <Dialog open={!!editState} onOpenChange={(o) => !o && setEditState(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Edit {editState?.user?.username}</DialogTitle>
          </DialogHeader>
          {editState && (
            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label>Role</Label>
                <Select value={editState.role} onValueChange={(v) => v && setEditState({ ...editState, role: v })}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">User</SelectItem>
                    <SelectItem value="admin">Admin</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label>Quota (GB)</Label>
                <Input
                  type="number"
                  min="0"
                  step="0.1"
                  value={editState.quota}
                  onChange={(e) => setEditState({ ...editState, quota: e.target.value })}
                />
              </div>
              <div className="space-y-1.5">
                <Label>New password (optional)</Label>
                <Input type="password" placeholder="Leave blank to keep current"
                  value={editState.newPassword}
                  onChange={(e) => setEditState({ ...editState, newPassword: e.target.value })} />
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditState(null)}>Cancel</Button>
            <Button onClick={saveEdit}>Save changes</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Create user</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            {(["username", "email", "password"] as const).map((field) => (
              <div key={field} className="space-y-1.5">
                <Label className="capitalize">{field}</Label>
                <Input
                  type={field === "password" ? "password" : "text"}
                  value={newUser[field]}
                  onChange={(e) => setNewUser({ ...newUser, [field]: e.target.value })}
                />
              </div>
            ))}
            <div className="space-y-1.5">
              <Label>Role</Label>
              <Select value={newUser.role} onValueChange={(v) => v && setNewUser({ ...newUser, role: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">User</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>Quota (GB)</Label>
              <Input
                type="number"
                min="0"
                step="0.1"
                value={newUser.quota}
                onChange={(e) => setNewUser({ ...newUser, quota: e.target.value })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button onClick={createUser}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </TooltipProvider>
  );
}
