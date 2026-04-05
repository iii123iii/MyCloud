"use client";

import { useState } from "react";
import { toast } from "sonner";
import { UserPlus, Pencil, Trash2, ShieldCheck, ShieldOff } from "lucide-react";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

import { admin as adminApi } from "@/lib/api";
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
  const [newUser, setNewUser] = useState({ username: "", email: "", password: "", role: "user", quota: "10737418240" });

  const toggleActive = async (user: User) => {
    try {
      await adminApi.updateUser(user.id, { is_active: !user.is_active });
      onMutate();
      toast.success(user.is_active ? "User disabled" : "User enabled");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to update user");
    }
  };

  const deleteUser = async (user: User) => {
    if (!confirm(`Delete user "${user.username}"? This cannot be undone.`)) return;
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
        quota_bytes: parseInt(editState.quota),
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
        quota_bytes: parseInt(newUser.quota),
      } as User & { password: string });
      onMutate();
      toast.success("User created");
      setCreateOpen(false);
      setNewUser({ username: "", email: "", password: "", role: "user", quota: "10737418240" });
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create user");
    }
  };

  return (
    <>
      <div className="flex justify-end mb-3">
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <UserPlus className="h-4 w-4 mr-1.5" />
          New user
        </Button>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Username</TableHead>
              <TableHead>Email</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Storage</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Joined</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((user) => (
              <TableRow key={user.id}>
                <TableCell className="font-medium">{user.username}</TableCell>
                <TableCell className="text-muted-foreground text-sm">{user.email}</TableCell>
                <TableCell>
                  <Badge variant={user.role === "admin" ? "default" : "secondary"}>
                    {user.role}
                  </Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {formatBytes(user.used_bytes)} / {formatBytes(user.quota_bytes)}
                </TableCell>
                <TableCell>
                  <Badge variant={user.is_active ? "outline" : "destructive"}>
                    {user.is_active ? "Active" : "Disabled"}
                  </Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">{formatDate(user.created_at)}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <Button variant="ghost" size="icon" className="h-7 w-7"
                      onClick={() => setEditState({ user, newPassword: "", quota: String(user.quota_bytes), role: user.role })}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => toggleActive(user)}>
                      {user.is_active
                        ? <ShieldOff className="h-3.5 w-3.5" />
                        : <ShieldCheck className="h-3.5 w-3.5" />
                      }
                    </Button>
                    <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive hover:text-destructive"
                      onClick={() => deleteUser(user)}>
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
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
                <Label>Quota (bytes)</Label>
                <Input value={editState.quota} onChange={(e) => setEditState({ ...editState, quota: e.target.value })} />
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
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button onClick={createUser}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
