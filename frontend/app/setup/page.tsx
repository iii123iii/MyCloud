"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  Cloud,
  Shield,
  Lock,
  Loader2,
  Eye,
  EyeOff,
  Check,
  CircleAlert,
  CheckCircle2,
  Sparkles,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { auth, ApiError } from "@/lib/api";

const schema = z
  .object({
    username: z.string().min(3, "Username must be at least 3 characters"),
    email: z.string().email("Please enter a valid email address"),
    password: z.string().min(8, "Password must be at least 8 characters"),
    confirmPassword: z.string(),
  })
  .refine((d) => d.password === d.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  });
type FormData = z.infer<typeof schema>;

function passwordStrength(pw: string): { score: number; label: string; tone: string } {
  let score = 0;
  if (pw.length >= 8) score++;
  if (pw.length >= 12) score++;
  if (/[A-Z]/.test(pw) && /[a-z]/.test(pw)) score++;
  if (/\d/.test(pw)) score++;
  if (/[^A-Za-z0-9]/.test(pw)) score++;
  const tiers = [
    { label: "Too short", tone: "bg-muted" },
    { label: "Weak", tone: "bg-destructive" },
    { label: "Fair", tone: "bg-amber-500" },
    { label: "Good", tone: "bg-yellow-500" },
    { label: "Strong", tone: "bg-emerald-500" },
    { label: "Excellent", tone: "bg-emerald-500" },
  ];
  const t = tiers[Math.min(score, tiers.length - 1)];
  return { score, label: t.label, tone: t.tone };
}

export default function SetupPage() {
  const router = useRouter();
  const [checking, setChecking] = useState(true);
  const [done, setDone] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const formTitleId = useId();
  const strengthId = useId();
  const submitErrorId = useId();
  const doneHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const usernameRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    auth
      .setupStatus()
      .then(({ setup_complete }) => {
        if (setup_complete) router.replace("/login");
        else setChecking(false);
      })
      .catch(() => setChecking(false));
  }, [router]);

  const {
    register,
    handleSubmit,
    watch,
    formState: { errors, isSubmitting, touchedFields, dirtyFields },
  } = useForm<FormData>({
    resolver: zodResolver(schema),
    mode: "onBlur",
  });

  // Focus the first field once the form is shown.
  useEffect(() => {
    if (!checking && !done) {
      usernameRef.current?.focus();
    }
  }, [checking, done]);

  // Move focus to the success heading for screen readers.
  useEffect(() => {
    if (done) {
      doneHeadingRef.current?.focus();
    }
  }, [done]);

  const passwordValue = watch("password") ?? "";
  const strength = passwordStrength(passwordValue);

  const onSubmit = async (data: FormData) => {
    setSubmitError(null);
    try {
      await auth.setupComplete({
        username: data.username,
        email: data.email,
        password: data.password,
      });
      setDone(true);
      toast.success("Setup complete. Redirecting to sign in…");
      setTimeout(() => router.push("/login"), 1400);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : "Setup failed";
      setSubmitError(message);
      toast.error(message);
    }
  };

  if (checking) {
    return (
      <div className="relative min-h-screen overflow-hidden bg-background">
        <BackgroundDecor />
        <div
          className="relative flex min-h-screen items-center justify-center px-4 py-10 sm:py-14"
          aria-busy="true"
          aria-live="polite"
        >
          <div className="w-full max-w-lg">
            <span className="sr-only">Checking setup status…</span>

            {/* Brand header skeleton */}
            <div className="mb-8 flex flex-col items-center text-center">
              <Skeleton className="mb-4 h-14 w-14 rounded-2xl" />
              <Skeleton className="h-9 w-40" />
              <Skeleton className="mt-3 h-6 w-32 rounded-full" />
              <Skeleton className="mt-4 h-4 w-72" />
            </div>

            {/* Trust badges skeleton */}
            <div className="mb-6 grid grid-cols-2 gap-3">
              <Skeleton className="h-16 w-full rounded-xl" />
              <Skeleton className="h-16 w-full rounded-xl" />
            </div>

            {/* Card skeleton */}
            <Card className="shadow-xl shadow-foreground/5">
              <CardHeader>
                <Skeleton className="h-5 w-48" />
                <Skeleton className="mt-2 h-4 w-full" />
              </CardHeader>
              <CardContent className="space-y-4">
                {[0, 1, 2, 3].map((i) => (
                  <div key={i} className="space-y-1.5">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-8 w-full" />
                  </div>
                ))}
                <Skeleton className="h-9 w-full" />
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    );
  }

  if (done) {
    return (
      <div className="relative min-h-screen overflow-hidden bg-background">
        <BackgroundDecor />
        <div
          className="relative flex min-h-screen items-center justify-center p-4"
          aria-live="polite"
        >
          <div className="animate-in fade-in zoom-in-95 duration-500 flex flex-col items-center gap-5 text-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-emerald-500/10 ring-1 ring-emerald-500/20">
              <CheckCircle2 className="h-8 w-8 text-emerald-500" aria-hidden />
            </div>
            <div className="space-y-1.5">
              <h1
                ref={doneHeadingRef}
                tabIndex={-1}
                className="font-heading text-2xl font-semibold tracking-tight outline-none focus-visible:ring-2 focus-visible:ring-ring/50 rounded sm:text-3xl"
              >
                You&apos;re all set
              </h1>
              <p className="text-sm text-muted-foreground sm:text-base">
                Redirecting you to sign in…
              </p>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Per-field validation visual helpers
  const fieldState = (name: keyof FormData) => {
    const hasError = !!errors[name];
    const isTouched = touchedFields[name];
    const isDirty = dirtyFields[name];
    if (hasError) return "error" as const;
    if (isTouched && isDirty) return "valid" as const;
    return "idle" as const;
  };

  const { ref: registerUsernameRef, ...usernameRegister } = register("username");

  return (
    <div className="relative min-h-screen overflow-hidden bg-background">
      <BackgroundDecor />

      <div className="relative flex min-h-screen items-center justify-center px-4 py-12 sm:py-16">
        <div className="w-full max-w-lg">
          {/* Brand header */}
          <header className="animate-in fade-in slide-in-from-bottom-4 duration-500 mb-10 flex flex-col items-center text-center">
            <div className="relative mb-5">
              <div
                aria-hidden
                className="absolute inset-0 -z-10 rounded-2xl bg-primary/20 blur-2xl"
              />
              <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-primary to-primary/70 text-primary-foreground shadow-lg shadow-primary/20 ring-1 ring-foreground/10 transition-transform duration-300 hover:scale-105">
                <Cloud className="h-7 w-7" strokeWidth={2.25} aria-hidden />
              </div>
            </div>
            <h1 className="font-heading text-3xl font-semibold tracking-tight sm:text-4xl">
              MyCloud
            </h1>
            <div className="mt-3 inline-flex items-center gap-1.5 rounded-full border border-foreground/10 bg-card/60 px-3 py-1 text-xs text-muted-foreground backdrop-blur">
              <Sparkles className="h-3 w-3" aria-hidden />
              <span>First-run setup</span>
            </div>
            <p className="mt-5 max-w-sm text-balance text-sm leading-relaxed text-muted-foreground sm:text-base">
              Welcome. Let&apos;s create the administrator account for your
              private cloud.
            </p>
          </header>

          {/* Trust badges */}
          <div
            className="animate-in fade-in slide-in-from-bottom-4 duration-500 mb-7 grid grid-cols-2 gap-3"
            style={{ animationDelay: "80ms", animationFillMode: "both" }}
          >
            {[
              {
                icon: Shield,
                label: "AES-256-GCM",
                sub: "Encrypted at rest",
              },
              {
                icon: Lock,
                label: "TLS in transit",
                sub: "HTTPS everywhere",
              },
            ].map(({ icon: Icon, label, sub }) => (
              <div
                key={label}
                className="group flex items-start gap-3 rounded-xl border border-foreground/10 bg-card/60 p-3 backdrop-blur transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:bg-card hover:shadow-md"
              >
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary ring-1 ring-primary/15 transition-colors group-hover:bg-primary/15">
                  <Icon className="h-4 w-4" aria-hidden />
                </div>
                <div className="min-w-0 space-y-0.5">
                  <p className="truncate text-sm font-medium leading-none">
                    {label}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {sub}
                  </p>
                </div>
              </div>
            ))}
          </div>

          {/* Form card */}
          <Card
            className="animate-in fade-in slide-in-from-bottom-4 duration-500 shadow-xl shadow-foreground/5 transition-shadow duration-300 hover:shadow-2xl hover:shadow-foreground/10"
            style={
              {
                animationDelay: "160ms",
                animationFillMode: "both",
              } as React.CSSProperties
            }
          >
            <CardHeader>
              <CardTitle id={formTitleId} className="text-lg sm:text-xl">
                Create admin account
              </CardTitle>
              <CardDescription className="leading-relaxed">
                You&apos;ll be the sole administrator. Additional users can be
                invited later from Settings.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form
                onSubmit={handleSubmit(onSubmit)}
                className="space-y-5"
                noValidate
                aria-labelledby={formTitleId}
                aria-describedby={submitError ? submitErrorId : undefined}
              >
                {/* Async submit error */}
                <div aria-live="polite" aria-atomic="true">
                  {submitError && (
                    <Alert
                      id={submitErrorId}
                      variant="destructive"
                      className="animate-in fade-in slide-in-from-top-1 duration-200"
                    >
                      <CircleAlert aria-hidden />
                      <AlertTitle>Couldn&apos;t complete setup</AlertTitle>
                      <AlertDescription>{submitError}</AlertDescription>
                    </Alert>
                  )}
                </div>

                {/* Username */}
                <FieldRow
                  id="username"
                  label="Username"
                  error={errors.username?.message}
                  state={fieldState("username")}
                >
                  <Input
                    id="username"
                    autoComplete="username"
                    placeholder="admin"
                    aria-invalid={!!errors.username || undefined}
                    aria-describedby={
                      errors.username ? "username-error" : undefined
                    }
                    disabled={isSubmitting}
                    {...usernameRegister}
                    ref={(el: HTMLInputElement | null) => {
                      registerUsernameRef(el);
                      usernameRef.current = el;
                    }}
                  />
                </FieldRow>

                {/* Email */}
                <FieldRow
                  id="email"
                  label="Email"
                  error={errors.email?.message}
                  state={fieldState("email")}
                >
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    inputMode="email"
                    placeholder="admin@example.com"
                    aria-invalid={!!errors.email || undefined}
                    aria-describedby={
                      errors.email ? "email-error" : undefined
                    }
                    disabled={isSubmitting}
                    {...register("email")}
                  />
                </FieldRow>

                {/* Password */}
                <FieldRow
                  id="password"
                  label="Password"
                  error={errors.password?.message}
                  state={fieldState("password")}
                >
                  <div className="relative">
                    <Input
                      id="password"
                      type={showPassword ? "text" : "password"}
                      autoComplete="new-password"
                      placeholder="At least 8 characters"
                      className="pr-10"
                      aria-invalid={!!errors.password || undefined}
                      aria-describedby={
                        [
                          errors.password ? "password-error" : null,
                          passwordValue.length > 0 ? strengthId : null,
                        ]
                          .filter(Boolean)
                          .join(" ") || undefined
                      }
                      disabled={isSubmitting}
                      {...register("password")}
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword((v) => !v)}
                      aria-label={
                        showPassword ? "Hide password" : "Show password"
                      }
                      aria-pressed={showPassword}
                      aria-controls="password"
                      disabled={isSubmitting}
                      className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
                    >
                      {showPassword ? (
                        <EyeOff className="h-4 w-4" aria-hidden />
                      ) : (
                        <Eye className="h-4 w-4" aria-hidden />
                      )}
                    </button>
                  </div>
                  {passwordValue.length > 0 && (
                    <div
                      id={strengthId}
                      className="mt-2 space-y-1.5 animate-in fade-in duration-200"
                    >
                      <div
                        className="flex gap-1"
                        role="meter"
                        aria-label="Password strength"
                        aria-valuemin={0}
                        aria-valuemax={5}
                        aria-valuenow={strength.score}
                        aria-valuetext={strength.label}
                      >
                        {[0, 1, 2, 3, 4].map((i) => (
                          <div
                            key={i}
                            aria-hidden
                            className={cn(
                              "h-1 flex-1 rounded-full transition-colors duration-300",
                              i < strength.score ? strength.tone : "bg-muted"
                            )}
                          />
                        ))}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Strength:{" "}
                        <span className="font-medium text-foreground transition-colors">
                          {strength.label}
                        </span>
                      </p>
                    </div>
                  )}
                </FieldRow>

                {/* Confirm */}
                <FieldRow
                  id="confirmPassword"
                  label="Confirm password"
                  error={errors.confirmPassword?.message}
                  state={fieldState("confirmPassword")}
                >
                  <div className="relative">
                    <Input
                      id="confirmPassword"
                      type={showConfirm ? "text" : "password"}
                      autoComplete="new-password"
                      placeholder="Re-enter password"
                      className="pr-10"
                      aria-invalid={!!errors.confirmPassword || undefined}
                      aria-describedby={
                        errors.confirmPassword
                          ? "confirmPassword-error"
                          : undefined
                      }
                      disabled={isSubmitting}
                      {...register("confirmPassword")}
                    />
                    <button
                      type="button"
                      onClick={() => setShowConfirm((v) => !v)}
                      aria-label={
                        showConfirm ? "Hide password" : "Show password"
                      }
                      aria-pressed={showConfirm}
                      aria-controls="confirmPassword"
                      disabled={isSubmitting}
                      className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
                    >
                      {showConfirm ? (
                        <EyeOff className="h-4 w-4" aria-hidden />
                      ) : (
                        <Eye className="h-4 w-4" aria-hidden />
                      )}
                    </button>
                  </div>
                </FieldRow>

                <Button
                  type="submit"
                  size="lg"
                  className="w-full transition-all duration-200 hover:brightness-110 hover:shadow-md hover:shadow-primary/20"
                  disabled={isSubmitting}
                  aria-busy={isSubmitting}
                >
                  {isSubmitting ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
                      <span>Setting up…</span>
                    </>
                  ) : (
                    "Complete setup"
                  )}
                </Button>

                <p className="pt-1 text-center text-xs text-muted-foreground">
                  By continuing you agree to host and manage your own data.
                </p>
              </form>
            </CardContent>
          </Card>

          <p
            className="animate-in fade-in duration-500 mt-7 text-center text-xs text-muted-foreground"
            style={
              {
                animationDelay: "240ms",
                animationFillMode: "both",
              } as React.CSSProperties
            }
          >
            MyCloud runs entirely on your infrastructure. Nothing leaves your
            server.
          </p>
        </div>
      </div>
    </div>
  );
}

/* ---------- helpers ---------- */

function FieldRow({
  id,
  label,
  error,
  state,
  children,
}: {
  id: string;
  label: string;
  error?: string;
  state: "idle" | "valid" | "error";
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor={id} className="text-sm font-medium">
          {label}
        </Label>
        {state === "valid" && (
          <span
            className="inline-flex items-center gap-1 text-xs text-emerald-500 animate-in fade-in duration-200"
            aria-hidden
          >
            <Check className="h-3 w-3" />
            Looks good
          </span>
        )}
      </div>
      {children}
      <div aria-live="polite" aria-atomic="true">
        {error && (
          <p
            id={`${id}-error`}
            className="animate-in fade-in slide-in-from-top-1 duration-200 flex items-start gap-1 text-xs text-destructive"
          >
            <CircleAlert className="mt-0.5 h-3 w-3 shrink-0" aria-hidden />
            <span>{error}</span>
          </p>
        )}
      </div>
    </div>
  );
}

function BackgroundDecor() {
  return (
    <>
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(ellipse_at_top,theme(colors.primary/0.12),transparent_55%),radial-gradient(ellipse_at_bottom,theme(colors.primary/0.08),transparent_60%)]"
      />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 -z-10 opacity-[0.35] [background-image:linear-gradient(to_right,theme(colors.foreground/0.04)_1px,transparent_1px),linear-gradient(to_bottom,theme(colors.foreground/0.04)_1px,transparent_1px)] [background-size:32px_32px] [mask-image:radial-gradient(ellipse_at_center,black,transparent_70%)]"
      />
    </>
  );
}
