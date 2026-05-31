"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  AlertTriangle,
  Check,
  KeyRound,
  Loader2,
  Activity as ActivityIcon,
  Code2,
  ShieldAlert,
  Settings,
  Smartphone,
  Trash2,
  User2,
} from "lucide-react";

import { auth as authApi, tokenStore } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ActivityLogTable } from "@/components/admin/ActivityLogTable";
import { SessionsView } from "./sessions/page";
import { DeveloperView } from "./developer/page";
import { SectionHeader } from "@/components/settings/SectionHeader";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "@/components/ui/dialog";
import { formatBytes, parseServerDate } from "@/lib/format";
import { cn } from "@/lib/utils";

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
    message: 'You must type DELETE (all caps)',
  }),
});
type DeleteForm = {
  password: string;
  confirm: string;
};

type SaveState = "idle" | "saving" | "saved";

const TAB_ITEMS = [
  { value: "profile",  label: "Profile",  icon: User2 },
  { value: "password", label: "Password", icon: KeyRound },
  { value: "activity", label: "Activity", icon: ActivityIcon },
  { value: "sessions", label: "Sessions", icon: Smartphone },
  { value: "developer", label: "Developer", icon: Code2 },
] as const;

export default function SettingsPage() {
  const router = useRouter();
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [pwSaveState, setPwSaveState] = useState<SaveState>("idle");
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

  // Drop the "Saved" state back to idle after a short delay so the button
  // doesn't sit stuck on the confirmation forever.
  useEffect(() => {
    if (pwSaveState !== "saved") return;
    const t = setTimeout(() => setPwSaveState("idle"), 2000);
    return () => clearTimeout(t);
  }, [pwSaveState]);

  const onPasswordChange = async (data: PwForm) => {
    setPwSaveState("saving");
    try {
      await authApi.changePassword({ old_password: data.old_password, new_password: data.new_password });
      toast.success("Password changed successfully");
      reset();
      setPwSaveState("saved");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to change password");
      setPwSaveState("idle");
    }
  };

  const onDeleteAccount = async (data: DeleteForm) => {
    try {
      await authApi.deleteAccount({ password: data.password });
      tokenStore.clear();
      resetDelete();
      setDeleteDialogOpen(false);
      toast.success("Account deleted");
      router.replace("/login");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to delete account");
    }
  };

  const usedPct = user && user.quota_bytes > 0
    ? Math.min(100, Math.round((user.used_bytes / user.quota_bytes) * 100))
    : 0;
  const overQuota = usedPct >= 90;

  return (
    <div className="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto space-y-6">
      <header className="flex items-center gap-2.5">
        <Settings className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
        <div className="space-y-0.5">
          <h1 className="text-xl font-semibold leading-tight tracking-tight">Settings</h1>
          <p className="text-sm text-muted-foreground">
            Manage your account, security, and activity.
          </p>
        </div>
      </header>

      {/*
        Tabs default to "horizontal" so the list stacks on top — perfect for
        small screens. From `md` up we promote them to "vertical" so the
        navigation lives on the left like a settings rail.
      */}
      <Tabs
        defaultValue="profile"
        orientation="horizontal"
        className="md:!flex-row md:gap-8 data-vertical:md:!flex-row"
      >
        <TabsList
          variant="line"
          className="w-full overflow-x-auto md:!flex-col md:w-56 md:h-fit md:items-stretch md:gap-0.5 md:p-1 md:bg-muted/40 md:rounded-lg"
        >
          {TAB_ITEMS.map(({ value, label, icon: Icon }) => (
            <TabsTrigger
              key={value}
              value={value}
              className="transition-colors md:w-full md:flex-none md:justify-start md:h-9 md:px-3 md:after:hidden md:data-active:bg-background md:data-active:shadow-sm"
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
              {label}
            </TabsTrigger>
          ))}
          <Separator className="hidden md:block my-1" />
          <TabsTrigger
            value="danger"
            className="transition-colors md:w-full md:flex-none md:justify-start md:h-9 md:px-3 md:after:hidden text-destructive hover:text-destructive md:data-active:bg-destructive/10 md:data-active:text-destructive"
          >
            <ShieldAlert className="h-4 w-4" aria-hidden="true" />
            Danger zone
          </TabsTrigger>
        </TabsList>

        <div className="flex-1 min-w-0 space-y-6">
          {/* ─── Profile ─── */}
          <TabsContent value="profile" className="space-y-6 mt-0">
            <SectionHeader
              title="Profile"
              description="Your account details and storage usage."
            />

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <User2 className="h-4 w-4" aria-hidden="true" />
                  Account details
                </CardTitle>
                <CardDescription>Read-only information about your account.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {user ? (
                  <dl className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-4 text-sm">
                    <ProfileRow label="Username" value={user.username} />
                    <ProfileRow label="Email" value={user.email} />
                    <ProfileRow label="Role" value={<span className="capitalize">{user.role}</span>} />
                    <ProfileRow
                      label="Member since"
                      value={parseValidDate(user.created_at)}
                    />
                  </dl>
                ) : (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-4" aria-busy="true" aria-label="Loading account details">
                    <div className="space-y-1.5">
                      <Skeleton className="h-3 w-16" />
                      <Skeleton className="h-4 w-32" />
                    </div>
                    <div className="space-y-1.5">
                      <Skeleton className="h-3 w-16" />
                      <Skeleton className="h-4 w-40" />
                    </div>
                    <div className="space-y-1.5">
                      <Skeleton className="h-3 w-16" />
                      <Skeleton className="h-4 w-20" />
                    </div>
                    <div className="space-y-1.5">
                      <Skeleton className="h-3 w-20" />
                      <Skeleton className="h-4 w-28" />
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Storage</CardTitle>
                <CardDescription>How much of your quota is currently in use.</CardDescription>
              </CardHeader>
              <CardContent>
                {user ? (
                  <div className="space-y-3">
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="text-2xl font-semibold tabular-nums leading-none">
                        {formatBytes(user.used_bytes)}
                      </span>
                      <span className="text-sm text-muted-foreground tabular-nums">
                        of {formatBytes(user.quota_bytes)}
                      </span>
                    </div>
                    <Progress
                      value={usedPct}
                      className={cn(
                        "h-2 transition-colors",
                        overQuota && "[&>div]:bg-destructive",
                      )}
                      aria-label={`Storage used: ${usedPct} percent`}
                    />
                    <div className="flex items-center justify-between text-xs">
                      <span className={cn(
                        "tabular-nums transition-colors",
                        overQuota ? "text-destructive font-medium" : "text-muted-foreground",
                      )}>
                        {usedPct}% used
                      </span>
                      <span className="text-muted-foreground tabular-nums">
                        {formatBytes(Math.max(0, user.quota_bytes - user.used_bytes))} free
                      </span>
                    </div>
                    {overQuota && (
                      <Alert variant="destructive" className="mt-2" role="alert">
                        <AlertTriangle className="h-4 w-4" aria-hidden="true" />
                        <AlertTitle>Approaching your quota</AlertTitle>
                        <AlertDescription>
                          Free up space by removing files from the trash or transferring ownership.
                        </AlertDescription>
                      </Alert>
                    )}
                  </div>
                ) : (
                  <div className="space-y-3" aria-busy="true" aria-label="Loading storage usage">
                    <div className="flex items-baseline justify-between gap-2">
                      <Skeleton className="h-7 w-24" />
                      <Skeleton className="h-4 w-16" />
                    </div>
                    <Skeleton className="h-2 w-full" />
                    <div className="flex items-center justify-between">
                      <Skeleton className="h-3 w-16" />
                      <Skeleton className="h-3 w-20" />
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          {/* ─── Password ─── */}
          <TabsContent value="password" className="space-y-6 mt-0">
            <SectionHeader
              title="Password"
              description="Choose a strong password that you don't use anywhere else."
            />
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <KeyRound className="h-4 w-4" aria-hidden="true" />
                  Change password
                </CardTitle>
                <CardDescription>
                  Use at least 8 characters. You will stay logged in on this device.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleSubmit(onPasswordChange)} className="space-y-5" noValidate>
                  <div className="space-y-1.5">
                    <Label htmlFor="old_password">Current password</Label>
                    <Input
                      id="old_password"
                      type="password"
                      autoComplete="current-password"
                      aria-invalid={errors.old_password ? "true" : undefined}
                      aria-describedby={errors.old_password ? "old_password-error" : undefined}
                      {...register("old_password")}
                    />
                    {errors.old_password && (
                      <p id="old_password-error" className="text-xs text-destructive" role="alert">
                        {errors.old_password.message}
                      </p>
                    )}
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <Label htmlFor="new_password">New password</Label>
                      <Input
                        id="new_password"
                        type="password"
                        autoComplete="new-password"
                        aria-invalid={errors.new_password ? "true" : undefined}
                        aria-describedby={cn(
                          "new_password-hint",
                          errors.new_password && "new_password-error",
                        )}
                        {...register("new_password")}
                      />
                      {errors.new_password ? (
                        <p id="new_password-error" className="text-xs text-destructive" role="alert">
                          {errors.new_password.message}
                        </p>
                      ) : (
                        <p id="new_password-hint" className="text-xs text-muted-foreground">
                          Minimum 8 characters.
                        </p>
                      )}
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="confirm">Confirm new password</Label>
                      <Input
                        id="confirm"
                        type="password"
                        autoComplete="new-password"
                        aria-invalid={errors.confirm ? "true" : undefined}
                        aria-describedby={errors.confirm ? "confirm-error" : undefined}
                        {...register("confirm")}
                      />
                      {errors.confirm && (
                        <p id="confirm-error" className="text-xs text-destructive" role="alert">
                          {errors.confirm.message}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center justify-end pt-1">
                    <SaveButton state={pwSaveState} submitting={isSubmitting} />
                  </div>
                  <p className="sr-only" role="status" aria-live="polite">
                    {pwSaveState === "saving"
                      ? "Saving password"
                      : pwSaveState === "saved"
                        ? "Password saved"
                        : ""}
                  </p>
                </form>
              </CardContent>
            </Card>

          </TabsContent>

          {/* ─── Sessions ─── */}
          <TabsContent value="sessions" className="space-y-6 mt-0">
            <SessionsView />
          </TabsContent>

          {/* ─── Developer ─── */}
          <TabsContent value="developer" className="space-y-6 mt-0">
            <DeveloperView />
          </TabsContent>

          {/* ─── Activity ─── */}
          <TabsContent value="activity" className="space-y-6 mt-0">
            <SectionHeader
              title="Activity"
              description="Recent actions taken on your account."
            />
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <ActivityIcon className="h-4 w-4" aria-hidden="true" />
                  Recent activity
                </CardTitle>
                <CardDescription>
                  The last 100 events recorded for your user.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ActivitySection />
              </CardContent>
            </Card>
          </TabsContent>

          {/* ─── Danger zone ─── */}
          <TabsContent value="danger" className="space-y-6 mt-0">
            <SectionHeader
              title="Danger zone"
              description="Irreversible actions. Read carefully before continuing."
            />
            <Card className="border-destructive/50 ring-destructive/30 bg-destructive/[0.02]">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-destructive">
                  <AlertTriangle className="h-4 w-4" aria-hidden="true" />
                  Delete account
                </CardTitle>
                <CardDescription>
                  Permanently remove your account along with all files, folders, shares, and trash.
                  This cannot be undone.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 rounded-lg border border-destructive/30 bg-background p-3 transition-colors">
                  <div className="text-sm space-y-0.5">
                    <p className="font-medium leading-tight">Delete this account</p>
                    <p className="text-muted-foreground">
                      All data will be wiped from the server immediately.
                    </p>
                  </div>
                  <Button
                    variant="destructive"
                    onClick={() => {
                      resetDelete();
                      setDeleteDialogOpen(true);
                    }}
                    aria-label="Delete account"
                  >
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    Delete account
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </div>
      </Tabs>

      {/* ── Delete-account confirmation dialog ── */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
              Delete your account
            </DialogTitle>
            <DialogDescription>
              This will permanently delete your account, all uploaded files, folders, shares, and
              trash. <strong>This cannot be undone.</strong>
            </DialogDescription>
          </DialogHeader>

          <Alert variant="destructive" className="mt-1" role="alert">
            <AlertTriangle className="h-4 w-4" aria-hidden="true" />
            <AlertTitle>You will lose everything</AlertTitle>
            <AlertDescription>
              All your data will be wiped from the server immediately.
            </AlertDescription>
          </Alert>

          <form onSubmit={handleDeleteSubmit(onDeleteAccount)} className="space-y-4 mt-2" noValidate>
            <div className="space-y-1.5">
              <Label htmlFor="delete_password">Enter your password</Label>
              <Input
                id="delete_password"
                type="password"
                autoComplete="current-password"
                aria-invalid={deleteErrors.password ? "true" : undefined}
                aria-describedby={deleteErrors.password ? "delete_password-error" : undefined}
                {...registerDelete("password")}
              />
              {deleteErrors.password && (
                <p id="delete_password-error" className="text-xs text-destructive" role="alert">
                  {deleteErrors.password.message}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="delete_confirm">
                Type <span className="font-mono font-bold tracking-widest">DELETE</span> to confirm
              </Label>
              <Input
                id="delete_confirm"
                autoComplete="off"
                placeholder="DELETE"
                className="font-mono tracking-widest"
                aria-invalid={deleteErrors.confirm ? "true" : undefined}
                aria-describedby={deleteErrors.confirm ? "delete_confirm-error" : "delete_confirm-hint"}
                {...registerDelete("confirm")}
              />
              {deleteErrors.confirm ? (
                <p id="delete_confirm-error" className="text-xs text-destructive" role="alert">
                  {deleteErrors.confirm.message}
                </p>
              ) : (
                <p id="delete_confirm-hint" className="text-xs text-muted-foreground">
                  Case-sensitive. Must be uppercase.
                </p>
              )}
            </div>

            <DialogFooter>
              <DialogClose render={<Button variant="outline" type="button" />}>
                Cancel
              </DialogClose>
              <Button type="submit" variant="destructive" disabled={isDeleting} aria-live="polite">
                {isDeleting ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                    Deleting…
                  </>
                ) : (
                  <>
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    Delete my account
                  </>
                )}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function ProfileRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-[0.7rem] uppercase tracking-wide text-muted-foreground font-medium">{label}</dt>
      <dd className="font-medium break-words leading-snug">{value}</dd>
    </div>
  );
}

function parseValidDate(value: string): string {
  const d = parseServerDate(value);
  return isNaN(d.getTime()) ? value : d.toLocaleDateString();
}

function SaveButton({ state, submitting }: { state: SaveState; submitting: boolean }) {
  const isSaving = state === "saving" || submitting;
  const isSaved = state === "saved";
  return (
    <Button
      type="submit"
      disabled={isSaving}
      aria-live="polite"
      className={cn(
        "min-w-36 transition-colors",
        isSaved && "bg-emerald-600 hover:bg-emerald-600 text-white",
      )}
    >
      {isSaving ? (
        <>
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          Saving…
        </>
      ) : isSaved ? (
        <>
          <Check className="h-4 w-4" aria-hidden="true" />
          Saved
        </>
      ) : (
        "Update password"
      )}
    </Button>
  );
}

function ActivitySection() {
  const { data, isLoading } = useSWR("me-activity", () => authApi.myActivity(100));
  if (isLoading) {
    return (
      <div className="space-y-2" aria-busy="true" aria-label="Loading activity">
        {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10 rounded" />)}
      </div>
    );
  }
  const logs = data?.logs ?? [];
  if (logs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-1.5 py-10 text-center">
        <ActivityIcon className="h-8 w-8 text-muted-foreground/40" aria-hidden="true" />
        <p className="text-sm font-medium">No activity yet</p>
        <p className="text-xs text-muted-foreground">
          Actions on your account will appear here.
        </p>
      </div>
    );
  }
  return <ActivityLogTable logs={logs} hideUserColumn />;
}
