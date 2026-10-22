Prefetch monitors upcoming missing TV episodes and requests searches through Sonarr when playback progresses in Silo.

- Prefetch starts at **10% playback progress** by default. Configure another percentage, or set 0 for immediate prefetch. Resuming or seeking past the threshold qualifies.
- Configure standard and optional 4K Sonarr instances, episodes ahead, user/profile filters, and Sonarr tag filters.
- Silo's internal API URL is discovered automatically. An administrator API key is still required.
- The administrator status page includes read-only connection checks and recent prefetch results.

Upload the ZIP matching your Silo server through **Administration → Plugins → Install from a file**, save settings, test connections, and turn on **Enable prefetch**. An explicitly saved threshold is preserved on upgrade; choose 10 in settings if it was previously saved as 0.

Packages are provided for Linux amd64, Linux arm64, and macOS arm64, with archive checksums in `SHA256SUMS`.

Tests cover the compiled plugin handshake, host discovery, early-stop suppression, progress threshold crossing, two Sonarr instances, episode windows, filters, and duplicate searches. Real playback on the initial Linux amd64 build was verified against Silo and Sonarr. The progress threshold is covered by automated tests; ARM and macOS packages are cross-compiled.
