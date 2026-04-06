#pragma once

// ── MyCloud version ───────────────────────────────────────────────────────────
//
// IMPORTANT — READ BEFORE EVERY RELEASE:
//
//   Every release requires manually bumping MYCLOUD_VERSION here before pushing
//   the release to GitHub. The value is embedded in the running binary and exposed via
//   GET /api/admin/updates/check so the frontend can compare against the latest
//   GitHub release tag.
//
// Release checklist:
//   1. Change MYCLOUD_VERSION below to the new version (e.g. "v1.1.0")
//   2. Commit and push the change to GitHub:
//        git add .
//        git commit -am "chore: bump version to v1.1.0"
//        git push origin main
//   3. Create a GitHub release tagged exactly "v1.1.0" at:
//        https://github.com/iii123iii/MyCloud/releases/new
//   4. Use the Admin Panel -> Updates tab on the server to check and apply it.
//      or use the Admin Panel → Updates tab to check and apply manually.
//
// Version format: "v<major>.<minor>.<patch>"
// The tag on GitHub must match this string exactly (including the leading "v").
//
#ifndef MYCLOUD_VERSION
#define MYCLOUD_VERSION     "v1.1.1"
#endif
#define MYCLOUD_GITHUB_REPO "iii123iii/MyCloud"
