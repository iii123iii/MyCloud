"use client";

import { useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { FileExplorer } from "@/components/file-explorer/FileExplorer";

// Thin client island for the shared-folder browser. You always enter via a
// specific shared folder, so a missing ?folder= means a stale/manual URL —
// send the user back to the Shared list. The FileExplorer itself also bounces
// here if the grant is revoked mid-browse (path fetch 404s).
export function SharedBrowseClient() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const folder = searchParams.get("folder");

  useEffect(() => {
    if (!folder) router.replace("/shared");
  }, [folder, router]);

  if (!folder) return null;

  return (
    <FileExplorer
      sharedRoot
      basePath="/shared/browse"
      rootCrumb={{ label: "Shared with me", href: "/shared" }}
    />
  );
}
