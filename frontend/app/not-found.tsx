// Custom 404. Next.js renders this for any route that doesn't match, plus
// for explicit `notFound()` calls from server components. Designed to match
// the ambient gradient + glassmorphism vibe of the auth screens so users
// don't feel dropped into a default error page.

import Link from "next/link";
import { CloudOff, ArrowLeft, Home } from "lucide-react";

import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <div className="relative min-h-screen flex items-center justify-center bg-background p-6 overflow-hidden">
      {/* Ambient gradient — mirrors the auth screens for a consistent feel
          on any landing-style page. */}
      <div className="pointer-events-none absolute inset-0 -z-10" aria-hidden="true">
        <div className="absolute -top-24 -left-24 size-96 rounded-full bg-primary/15 blur-3xl" />
        <div className="absolute -bottom-32 -right-24 size-[28rem] rounded-full bg-primary/10 blur-3xl" />
        <div className="absolute inset-0 bg-gradient-to-b from-transparent via-background/0 to-background/30" />
      </div>

      <div className="w-full max-w-md text-center space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-500">
        <div className="mx-auto flex h-20 w-20 items-center justify-center rounded-2xl bg-background shadow-lg ring-1 ring-border">
          <CloudOff className="size-10 text-muted-foreground" strokeWidth={1.5} />
        </div>

        <div className="space-y-2">
          <p className="text-sm font-semibold uppercase tracking-widest text-primary">
            404
          </p>
          <h1 className="text-3xl sm:text-4xl font-semibold tracking-tight">
            We couldn&apos;t find that page
          </h1>
          <p className="text-muted-foreground max-w-sm mx-auto">
            The link might be broken, or the file you&apos;re looking for has been
            moved or deleted.
          </p>
        </div>

        <div className="flex flex-col sm:flex-row items-center justify-center gap-2">
          {/* This Button uses a base-ui style `render` prop instead of
              shadcn-classic `asChild` — see other call sites in the app. */}
          <Button size="lg" className="h-11 px-6" render={<Link href="/dashboard" />}>
            <Home className="size-4 mr-2" />
            Back to My Files
          </Button>
          <Button variant="outline" size="lg" className="h-11 px-6" render={<Link href="/login" />}>
            <ArrowLeft className="size-4 mr-2" />
            Sign in
          </Button>
        </div>

        <p className="text-xs text-muted-foreground/70">
          Tip: press <kbd className="rounded border bg-muted px-1.5 py-0.5 text-[10px] font-mono">Ctrl</kbd> +{" "}
          <kbd className="rounded border bg-muted px-1.5 py-0.5 text-[10px] font-mono">K</kbd> anywhere in MyCloud to jump.
        </p>
      </div>
    </div>
  );
}
