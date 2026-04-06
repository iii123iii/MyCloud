"use client";

import useSWR from "swr";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { AlertTriangle, Settings, User2 } from "lucide-react";

import { auth as authApi, tokenStore } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { formatBytes } from "@/lib/format";

const pwSchema = z.object({
  old_password: z.string().min(1, "Current password required"),
  new_password: z.string().min(8, "Min 8 characters"),
  confirm: z.string(),
}).refine((d) => d.new_password === d.confirm, {
  message: "Passwords do not match",
  path: ["confirm"],
});
type PwForm = z.infer<typeof pwSchema>;

const deleteSchema = z.object({
  password: z.string().min(1, "Current password required"),
  confirm: z.string().refine((value) => value === "DELETE", {
    message: 'Type DELETE to confirm',
  }),
});
type DeleteForm = {
  password: string;
  confirm: string;
};

export default function SettingsPage() {
  const router = useRouter();
  const { data: user } = useSWR("me", authApi.me);
  const { register, handleSubmit, reset, formState: { errors, isSubmitting } } = useForm<PwForm>({
    resolver: zodResolver(pwSchema),
  });
  const {
    register: registerDelete,
    handleSubmit: handleDeleteSubmit,
    reset: resetDelete,
    formState: { errors: deleteErrors, isSubmitting: isDeleting },
  } = useForm<DeleteForm>({ resolver: zodResolver(deleteSchema) });

  const onPasswordChange = async (data: PwForm) => {
    try {
      await authApi.changePassword({ old_password: data.old_password, new_password: data.new_password });
      toast.success("Password changed successfully");
      reset();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to change password");
    }
  };

  const onDeleteAccount = async (data: DeleteForm) => {
    try {
      await authApi.deleteAccount({ password: data.password });
      tokenStore.clear();
      resetDelete();
      toast.success("Account deleted");
      router.replace("/login");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to delete account");
    }
  };

  const usedPct = user && user.quota_bytes > 0
    ? Math.min(100, Math.round((user.used_bytes / user.quota_bytes) * 100))
    : 0;

  return (
    <div className="p-6 max-w-2xl space-y-6">
      <div className="flex items-center gap-2">
        <Settings className="h-5 w-5" />
        <h1 className="text-xl font-semibold">Settings</h1>
      </div>

      {/* Profile */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <User2 className="h-4 w-4" />
            Profile
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {user ? (
            <>
              <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
                <span className="text-muted-foreground">Username</span>
                <span className="font-medium">{user.username}</span>
                <span className="text-muted-foreground">Email</span>
                <span className="font-medium">{user.email}</span>
                <span className="text-muted-foreground">Role</span>
                <span className="font-medium capitalize">{user.role}</span>
                <span className="text-muted-foreground">Member since</span>
                <span className="font-medium">{new Date(user.created_at).toLocaleDateString()}</span>
              </div>
              <Separator />
              <div className="space-y-1.5">
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">Storage used</span>
                  <span>{formatBytes(user.used_bytes)} / {formatBytes(user.quota_bytes)}</span>
                </div>
                <Progress value={usedPct} className="h-2" />
                <p className="text-xs text-muted-foreground">{usedPct}% of your quota used</p>
              </div>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">Loading…</p>
          )}
        </CardContent>
      </Card>

      {/* Change password */}
      <Card>
        <CardHeader>
          <CardTitle>Change password</CardTitle>
          <CardDescription>Update your account password.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onPasswordChange)} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="old_password">Current password</Label>
              <Input id="old_password" type="password" {...register("old_password")} />
              {errors.old_password && <p className="text-xs text-destructive">{errors.old_password.message}</p>}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="new_password">New password</Label>
              <Input id="new_password" type="password" {...register("new_password")} />
              {errors.new_password && <p className="text-xs text-destructive">{errors.new_password.message}</p>}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="confirm">Confirm new password</Label>
              <Input id="confirm" type="password" {...register("confirm")} />
              {errors.confirm && <p className="text-xs text-destructive">{errors.confirm.message}</p>}
            </div>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving…" : "Update password"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="border-destructive/40">
        <CardHeader>
          <CardTitle>Delete account</CardTitle>
          <CardDescription>
            Permanently remove your account and delete all of your files and folders.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>This action cannot be undone</AlertTitle>
            <AlertDescription>
              Your account, stored files, folders, shares, and trash will be permanently deleted.
            </AlertDescription>
          </Alert>

          <form onSubmit={handleDeleteSubmit(onDeleteAccount)} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="delete_password">Current password</Label>
              <Input id="delete_password" type="password" {...registerDelete("password")} />
              {deleteErrors.password && <p className="text-xs text-destructive">{deleteErrors.password.message}</p>}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="delete_confirm">Type DELETE to confirm</Label>
              <Input id="delete_confirm" autoComplete="off" {...registerDelete("confirm")} />
              {deleteErrors.confirm && <p className="text-xs text-destructive">{deleteErrors.confirm.message}</p>}
            </div>
            <Button type="submit" variant="destructive" disabled={isDeleting}>
              {isDeleting ? "Deleting account..." : "Delete account"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
