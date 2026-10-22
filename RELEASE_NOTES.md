Prefetch monitors upcoming missing TV episodes and requests searches through Sonarr when playback progresses in Silo.

Version 0.2.1 prepares the repository for community review:

- Adds CODEOWNERS, a security reporting policy, support details, and a community submission checklist.
- Publishes checksum-bearing per-platform manifests alongside the standard raw binaries and `checksums.txt`.
- Verifies executable architectures and agreement between raw binaries, manifests, ZIPs, and catalog entries before release.
- Adds the standard `plugin_release_published` catalog notification, using the actual repository name. Without `SILO_PLUGINS_DISPATCH_TOKEN`, notification is skipped and publication succeeds.
- Describes search requests and the playback threshold accurately in the manifest and status page.

Playback behavior remains:

- Prefetch starts at **10% playback progress** by default. Configure another percentage, or set 0 for immediate prefetch. Resuming or seeking past the threshold qualifies.
- Configure standard and optional 4K Sonarr instances, episodes ahead, user/profile filters, and Sonarr tag filters.
- Silo's internal API URL is discovered automatically. An administrator API key is still required.
- The administrator status page includes read-only connection checks and recent prefetch results.

Upload the ZIP matching your Silo server through **Administration → Plugins → Install from a file**, save settings, test connections, and turn on **Enable prefetch**. An explicitly saved threshold is preserved on upgrade; choose 10 in settings if it was previously saved as 0.

Packages are provided for Linux amd64, Linux arm64, and macOS arm64, with archive checksums in `SHA256SUMS` and raw binary checksums in `checksums.txt`.

This release is available through the custom repository and manual installation. Community approval, ownership transfer, and a maintainer's live review are still pending.

Tests cover the compiled plugin handshake, host discovery, early-stop suppression, progress threshold crossing, two Sonarr instances, episode windows, filters, and duplicate searches. Real playback on the initial Linux amd64 build was verified against Silo and Sonarr. The progress threshold is covered by automated tests; ARM and macOS packages are cross-compiled.
