import { redirect } from "next/navigation";

// Root redirects to dashboard; the dashboard will redirect to /login if not authed
export default function Home() {
  redirect("/dashboard");
}
