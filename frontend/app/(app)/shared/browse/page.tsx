// Browse INTO a folder shared with the current user. Reuses the drive's
// FileExplorer in "shared" mode (read-only or editable depending on the grant
// level), with a "Shared with me" breadcrumb root. Entry is from the Shared
// page's incoming-grants list: /shared → folder row → /shared/browse?folder=<id>.
//
// Server shell + Suspense mirrors the dashboard so the client island
// (SharedBrowseClient, which calls useSearchParams) has its required boundary.
import { Suspense } from "react";
import { SharedBrowseClient } from "./SharedBrowseClient";

export default function SharedBrowsePage() {
  return (
    <div className="p-4 sm:p-6 lg:p-8">
      <h1 className="sr-only">Shared folder</h1>
      <Suspense>
        <SharedBrowseClient />
      </Suspense>
    </div>
  );
}
