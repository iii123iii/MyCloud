"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Cloud, Shield, Lock } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { auth, ApiError } from "@/lib/api";

const schema = z.object({
  username: z.string().min(3, "Username must be at least 3 characters"),
  email: z.string().email("Invalid email"),
  password: z.string().min(8, "Password must be at least 8 characters"),
  confirmPassword: z.string(),
}).refine((d) => d.password === d.confirmPassword, {
  message: "Passwords do not match",
  path: ["confirmPassword"],
});
type FormData = z.infer<typeof schema>;

export default function SetupPage() {
  const router = useRouter();
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    auth.setupStatus()
      .then(({ setup_complete }) => {
        if (setup_complete) router.replace("/login");
        else setChecking(false);
      })
      .catch(() => setChecking(false));
  }, [router]);

  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm<FormData>({
    resolver: zodResolver(schema),
  });

  const onSubmit = async (data: FormData) => {
    try {
      await auth.setupComplete({
        username: data.username,
        email: data.email,
        password: data.password,
      });
      toast.success("Setup complete! Please sign in with your new admin account.");
      router.push("/login");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Setup failed");
    }
  };

  if (checking) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-muted-foreground text-sm">Checking setup status…</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="w-full max-w-lg space-y-6">
        {/* Header */}
        <div className="text-center space-y-2">
          <div className="flex items-center justify-center gap-2">
            <Cloud className="h-8 w-8" />
            <span className="text-2xl font-bold">MyCloud</span>
          </div>
          <p className="text-muted-foreground">Initial setup — create your admin account</p>
        </div>

        {/* Feature highlights */}
        <div className="grid grid-cols-2 gap-3">
          {[
            { icon: Shield, label: "AES-256-GCM encryption", sub: "All files encrypted at rest" },
            { icon: Lock, label: "TLS in transit", sub: "Self-signed HTTPS certificate" },
          ].map(({ icon: Icon, label, sub }) => (
            <div key={label} className="border rounded-lg p-3 space-y-1">
              <div className="flex items-center gap-2 text-sm font-medium">
                <Icon className="h-4 w-4" />
                {label}
              </div>
              <p className="text-xs text-muted-foreground">{sub}</p>
            </div>
          ))}
        </div>

        <Separator />

        <Card>
          <CardHeader>
            <CardTitle>Create admin account</CardTitle>
            <CardDescription>
              This will be the only admin account. You can create additional users after setup.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="username">Username</Label>
                <Input id="username" {...register("username")} placeholder="admin" />
                {errors.username && <p className="text-xs text-destructive">{errors.username.message}</p>}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="email">Email</Label>
                <Input id="email" type="email" {...register("email")} placeholder="admin@example.com" />
                {errors.email && <p className="text-xs text-destructive">{errors.email.message}</p>}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="password">Password</Label>
                <Input id="password" type="password" {...register("password")} />
                {errors.password && <p className="text-xs text-destructive">{errors.password.message}</p>}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="confirmPassword">Confirm password</Label>
                <Input id="confirmPassword" type="password" {...register("confirmPassword")} />
                {errors.confirmPassword && (
                  <p className="text-xs text-destructive">{errors.confirmPassword.message}</p>
                )}
              </div>
              <Button type="submit" className="w-full" disabled={isSubmitting}>
                {isSubmitting ? "Setting up…" : "Complete setup"}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
