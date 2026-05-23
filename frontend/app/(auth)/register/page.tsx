"use client";

import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Eye, EyeOff } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { auth, ApiError } from "@/lib/api";

function safeNext(raw: string | null): string {
  if (!raw) return "/dashboard";
  if (!raw.startsWith("/") || raw.startsWith("//")) return "/dashboard";
  return raw;
}

const schema = z
  .object({
    username: z
      .string()
      .min(3, "At least 3 characters")
      .max(32, "32 characters max")
      .regex(/^[a-zA-Z0-9_.-]+$/, "Letters, numbers, . - _ only"),
    email: z.string().email("Invalid email"),
    password: z.string().min(8, "At least 8 characters"),
    confirmPassword: z.string(),
  })
  .refine((d) => d.password === d.confirmPassword, {
    message: "Doesn't match",
    path: ["confirmPassword"],
  });
type FormData = z.infer<typeof schema>;

export default function RegisterPage() {
  // useSearchParams forces a client-side bailout under Next 16's static
  // generation; the Suspense boundary lets the surrounding shell prerender.
  return (
    <Suspense>
      <RegisterForm />
    </Suspense>
  );
}

function RegisterForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextUrl = safeNext(searchParams.get("next"));
  const [showPass, setShowPass] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    setFocus,
    formState: { errors, isSubmitting },
  } = useForm<FormData>({ resolver: zodResolver(schema), mode: "onBlur" });

  const onInvalid = (fieldErrors: typeof errors) => {
    const first = (Object.keys(fieldErrors) as Array<keyof FormData>)[0];
    if (first) setFocus(first);
  };

  const onSubmit = async (data: FormData) => {
    setFormError(null);
    try {
      await auth.register({
        username: data.username,
        email: data.email,
        password: data.password,
      });
      toast.success("Account created.");
      router.push(nextUrl);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : "Registration failed";
      setFormError(message);
    }
  };

  const loginHref =
    nextUrl !== "/dashboard"
      ? `/login?next=${encodeURIComponent(nextUrl)}`
      : "/login";

  return (
    <main className="min-h-screen grid place-items-center p-6">
      <Card className="w-full max-w-[22rem] p-8 space-y-8">
        <div className="space-y-1.5">
          <p className="text-xs font-medium tracking-tight text-muted-foreground">
            mycloud
          </p>
          <h2 className="text-lg font-medium">Create account</h2>
        </div>

        <form
          onSubmit={handleSubmit(onSubmit, onInvalid)}
          className="space-y-6"
          noValidate
        >

          <div className="space-y-4">
            <Field
              id="username"
              label="Username"
              error={errors.username?.message}
              inputProps={{
                autoComplete: "username",
                autoCapitalize: "off",
                autoCorrect: "off",
                spellCheck: false,
                autoFocus: true,
                disabled: isSubmitting,
                ...register("username"),
              }}
            />

            <Field
              id="email"
              label="Email"
              error={errors.email?.message}
              inputProps={{
                type: "email",
                inputMode: "email",
                autoComplete: "email",
                autoCapitalize: "off",
                autoCorrect: "off",
                spellCheck: false,
                disabled: isSubmitting,
                ...register("email"),
              }}
            />

            <Field
              id="password"
              label="Password"
              error={errors.password?.message}
              hint="At least 8 characters."
              suffix={
                <button
                  type="button"
                  onClick={() => setShowPass((v) => !v)}
                  className="text-muted-foreground hover:text-foreground transition-colors"
                  aria-label={showPass ? "Hide password" : "Show password"}
                  aria-pressed={showPass}
                  tabIndex={0}
                >
                  {showPass ? (
                    <EyeOff className="h-4 w-4" aria-hidden="true" />
                  ) : (
                    <Eye className="h-4 w-4" aria-hidden="true" />
                  )}
                </button>
              }
              inputProps={{
                type: showPass ? "text" : "password",
                autoComplete: "new-password",
                disabled: isSubmitting,
                ...register("password"),
              }}
            />

            <Field
              id="confirmPassword"
              label="Confirm password"
              error={errors.confirmPassword?.message}
              suffix={
                <button
                  type="button"
                  onClick={() => setShowConfirm((v) => !v)}
                  className="text-muted-foreground hover:text-foreground transition-colors"
                  aria-label={showConfirm ? "Hide password" : "Show password"}
                  aria-pressed={showConfirm}
                  tabIndex={0}
                >
                  {showConfirm ? (
                    <EyeOff className="h-4 w-4" aria-hidden="true" />
                  ) : (
                    <Eye className="h-4 w-4" aria-hidden="true" />
                  )}
                </button>
              }
              inputProps={{
                type: showConfirm ? "text" : "password",
                autoComplete: "new-password",
                disabled: isSubmitting,
                ...register("confirmPassword"),
              }}
            />
          </div>

          <div aria-live="polite" aria-atomic="true" className="min-h-[1.25rem]">
            {formError && (
              <p role="alert" className="text-xs text-destructive">
                {formError}
              </p>
            )}
          </div>

          <Button
            type="submit"
            className="w-full"
            disabled={isSubmitting}
            aria-busy={isSubmitting}
          >
            {isSubmitting ? "Creating account…" : "Create account"}
          </Button>
        </form>

        <p className="text-xs text-muted-foreground">
          Already have an account?{" "}
          <Link
            href={loginHref}
            className="text-foreground underline underline-offset-4 hover:no-underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
          >
            Sign in
          </Link>
        </p>
      </Card>
    </main>
  );
}

function Field({
  id,
  label,
  error,
  hint,
  suffix,
  inputProps,
}: {
  id: string;
  label: string;
  error?: string;
  hint?: string;
  suffix?: React.ReactNode;
  inputProps: React.InputHTMLAttributes<HTMLInputElement>;
}) {
  const errorId = error ? `${id}-error` : undefined;
  const hintId = hint && !error ? `${id}-hint` : undefined;
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id} className="text-xs font-normal text-muted-foreground">
        {label}
      </Label>
      <div className="relative">
        <Input
          id={id}
          aria-invalid={error ? true : undefined}
          aria-describedby={cn(errorId, hintId) || undefined}
          className={cn(suffix && "pr-9")}
          {...inputProps}
        />
        {suffix && (
          <div className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center">
            {suffix}
          </div>
        )}
      </div>
      {error ? (
        <p id={errorId} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : hint ? (
        <p id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
