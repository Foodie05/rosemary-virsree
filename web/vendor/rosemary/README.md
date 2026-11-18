# Rosemary Forma SDK provenance

These unmodified tarballs are pinned to the official [Rosemary Design System v0.5.0 release](https://github.com/Foodie05/Rosemary-Design-System/releases/tag/v0.5.0), implementing [Forma 1.3.0](https://design.cruty.cn/). Downloaded 2026-11-18. Each tarball was verified against the publisher's attached `SHA256SUMS` (retained in full).

Both direct dependencies and transitive overrides use the same local tarballs; no private npm registry or build-time network fetch is required for the SDK. `pnpm install --frozen-lockfile` verifies lockfile integrity. To verify the vendor files independently, run from this directory:

```sh
awk '/rosemary-ui-.*tgz$/ {print}' SHA256SUMS | shasum -a 256 -c -
```

Code, tokens and functional icons are MIT licensed; see LICENSE. Rosemary branding is governed separately by BRAND-ASSETS.md. Arca uses the unchanged official PNG through RosemaryLogo, and retains Arca as the primary product name.
