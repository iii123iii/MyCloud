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

// Only allow same-origin relative paths to prevent ?next=https://evil.com
// open-redirects. Must start with a single "/" (not "//", which is
// protocol-relative).
function safeNext(raw: string | null): string {
  if (!raw) return "/dashboard";
  if (!raw.startsWith("/") || raw.startsWith("//")) return "/dashboard";
  return raw;
}

const schema = z.object({
  email: z.string().min(1, "Required"),
  password: z.string().min(1, "Required"),
});
type FormData = z.infer<typeof schema>;

// Map low-level API errors to copy that helps the user recover. 401 stays
// vague (don't reveal account existence); 429 surfaces the wait hint.
function friendlyAuthError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401 || err.code === "invalid_credentials") {
      return "Email or password is wrong.";
    }
    if (err.status === 429 || err.code === "rate_limited") {
      return err.message || "Too many attempts. Wait a moment.";
    }
    if (err.status === 0) return "Can't reach the server.";
    if (err.message) return err.message;
  }
  return "Sign-in failed.";
}

export default function LoginPage() {
  // useSearchParams forces a client-side bailout under Next 16's static
  // generation; wrapping the form in a Suspense boundary lets the surrounding
  // shell prerender while the search params resolve on the client.
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  // Where to send the user after a successful sign-in. Populated by the
  // app-shell auth guard (and friends) when they bounce an unauthenticated
  // visit to /login.
  const nextUrl = safeNext(searchParams.get("next"));
  const [showPass, setShowPass] = useState(false);
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
      const tokens = await auth.login(data);
      if (tokens.must_change_password) {
        toast.info("Change your password before continuing.");
        router.push("/settings?tab=password");
      } else {
        router.push(nextUrl);
      }
    } catch (err) {
      const message = friendlyAuthError(err);
      setFormError(message);
      setFocus("password");
    }
  };

  // Carry ?next through to the register link so a user who switches over
  // and creates an account still lands at the original destination.
  const registerHref =
    nextUrl !== "/dashboard"
      ? `/register?next=${encodeURIComponent(nextUrl)}`
      : "/register";

  return (
    <main className="min-h-screen grid place-items-center p-6">
      <Card className="w-full max-w-[22rem] p-8 space-y-8">
        <div className="space-y-1.5">
          <p className="text-xs font-medium tracking-tight text-muted-foreground">
            mycloud
          </p>
          <h2 className="text-lg font-medium">Sign in</h2>
        </div>

        <form
          onSubmit={handleSubmit(onSubmit, onInvalid)}
          className="space-y-6"
          noValidate
        >

          <div className="space-y-4">
            <Field
              id="email"
              label="Email or username"
              error={errors.email?.message}
              inputProps={{
                type: "text",
                inputMode: "email",
                autoComplete: "username",
                autoCapitalize: "off",
                autoCorrect: "off",
                spellCheck: false,
                autoFocus: true,
                disabled: isSubmitting,
                ...register("email"),
              }}
            />

            <Field
              id="password"
              label="Password"
              error={errors.password?.message}
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
                autoComplete: "current-password",
                disabled: isSubmitting,
                ...register("password"),
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
            {isSubmitting ? "Signing in…" : "Sign in"}
          </Button>
        </form>

        <p className="text-xs text-muted-foreground">
          No account?{" "}
          <Link
            href={registerHref}
            className="text-foreground underline underline-offset-4 hover:no-underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
          >
            Create one
          </Link>
        </p>
      </Card>
    </main>
  );
}

// Small helper so the field shape stays uniform between login/register
// without adding a shadcn-flavoured wrapper component to the codebase.
function Field({
  id,
  label,
  error,
  suffix,
  inputProps,
}: {
  id: string;
  label: string;
  error?: string;
  suffix?: React.ReactNode;
  inputProps: React.InputHTMLAttributes<HTMLInputElement>;
}) {
  const errorId = error ? `${id}-error` : undefined;
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id} className="text-xs font-normal text-muted-foreground">
        {label}
      </Label>
      <div className="relative">
        <Input
          id={id}
          aria-invalid={error ? true : undefined}
          aria-describedby={errorId}
          className={cn(suffix && "pr-9")}
          {...inputProps}
        />
        {suffix && (
          <div className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center">
            {suffix}
          </div>
        )}
      </div>
      {error && (
        <p id={errorId} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
