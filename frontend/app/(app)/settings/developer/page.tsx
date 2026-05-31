"use client";

// Settings → Developer: personal access tokens and outbound webhooks.
//
// Exported as a named DeveloperView so the parent settings page can embed it as
// a tab, plus a default export so /settings/developer works as a direct URL —
// the same dual-export pattern used by settings/sessions.

import { SectionHeader } from "@/components/settings/SectionHeader";
import { TokensCard } from "@/components/settings/TokensCard";
import { WebhooksCard } from "@/components/settings/WebhooksCard";

export function DeveloperView() {
  return (
    <div className="space-y-6">
      <SectionHeader
        title="Developer"
        description="Programmatic access to your account: API tokens and event webhooks."
      />
      <TokensCard />
      <WebhooksCard />
    </div>
  );
}

export default function DeveloperPage() {
  return (
    <div className="container max-w-4xl mx-auto py-8 px-4">
      <DeveloperView />
    </div>
  );
}
