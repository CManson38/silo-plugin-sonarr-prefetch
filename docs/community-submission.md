# Community plugin submission

This checklist follows the [approval policy](https://github.com/Silo-Community/silo-plugins/blob/main/APPROVAL_POLICY.md), [Scrob approval request #11](https://github.com/Silo-Community/silo-plugins/issues/11), and its [linked maintainer review](https://github.com/Joloxx9/silo-plugin-watchprovider-scrob/issues/2). Issue #11 is an approval request rather than a pull request. Its Scrob-specific playback and rating fixes do not apply to Prefetch; the ownership, release, documentation, and maintenance requirements do.

## Readiness checklist

| Requirement | Prefetch evidence / remaining work |
| --- | --- |
| Public source owned by Silo-Community | Public at `CManson38/silo-plugin-sonarr-prefetch`. Ownership transfer is pending; ask the community maintainers for the transfer process before moving it. |
| Builds and tests without local replacements | Pinned SDK version; no local `replace` directives. CI rejects local replacements, sets `GOWORK=off`, and runs tests with the race detector, vet, formatting, manifest introspection, and three platform builds. |
| Reviewer exercises setup and primary behavior | Earlier Linux amd64 playback and Sonarr monitoring/search results are recorded in [TESTING.md](../TESTING.md). A community reviewer still needs to test the submitted release, including the 10% threshold. ARM/macOS runtime execution has not been verified. |
| Accurate manifest and README | Documents Silo v2 admin sessions, administrator key, host URL discovery, progress threshold, both Sonarr instances, filters, monitoring/search actions, and network/credential handling. |
| No known unsafe or deceptive behavior | Redirects are refused; API keys travel in headers; upstream bodies and credentials are omitted from errors. Connection checks read services only. Administrator routes are declared in the manifest. Maintainer source/workflow review is still required. |
| Standard release assets and notification | `plugin-linux-amd64`, `plugin-linux-arm64`, `plugin-darwin-arm64`, `checksums.txt`, and checksum-bearing platform manifests. ZIPs and a standalone catalog are also available. `plugin_release_published` uses `github.repository` and the tag; the catalog dispatch secret must be supplied by an authorized maintainer. |
| License, support, maintainers | MIT license, README support link, `.github/CODEOWNERS` naming `@CManson38`, and private vulnerability reporting documented in `SECURITY.md`. |

## Submission draft

Title: **Approve community.requests.prefetch (Sonarr Prefetch)**

Requesting community approval for Sonarr Prefetch, a Silo plugin that monitors upcoming missing TV episodes and requests searches through Sonarr as playback progresses.

Repository: https://github.com/CManson38/silo-plugin-sonarr-prefetch

Release for review: https://github.com/CManson38/silo-plugin-sonarr-prefetch/releases/tag/v0.2.1

Plugin ID: `community.requests.prefetch` (kept stable for existing installations).

### Behavior and setup

- Subscribes to Silo session-change events and reads active sessions through `GET /api/v2/admin/sessions`. The Silo URL is discovered from the runtime host; an administrator-owned API key with empty scopes is required.
- Waits until the playing episode reaches 10% by default. The percentage is configurable; 0 starts immediately. Stopping below the threshold causes no monitoring or searches. Resuming or seeking past it qualifies.
- Checks a standard Sonarr instance and an optional second 4K instance wherever the series already exists. Applies user/profile and Sonarr tag inclusion/exclusion filters.
- Checks the next configured number of episodes across seasons, skipping specials. Monitors missing episodes and requests searches for aired episodes, skipping queued episodes and recent searches. Sonarr controls release selection and its download clients.
- Sends requests only to its discovered Silo server and configured Sonarr instances. No telemetry. Keys travel in headers, redirects are refused, and status/errors omit keys and upstream response bodies. The status and connection-check routes are administrator-only.

### Release and maintenance

The release workflow builds Linux amd64, Linux arm64, and macOS arm64 binaries, publishes `checksums.txt` and checksum-bearing platform manifests, and verifies the binaries, ZIPs, and catalog entries agree. CI also runs race tests, vet, formatting checks, and real compiled-plugin handshake/event tests without local dependency replacements.

The standard `plugin_release_published` notification uses `github.repository`, so the payload follows a transfer. It skips notification when `SILO_PLUGINS_DISPATCH_TOKEN` is absent. I would need an authorized catalog maintainer to arrange that secret after approval.

The project is MIT licensed. I maintain it as `@CManson38`; CODEOWNERS, a support path, and a security reporting policy are included.

### Test evidence and limits

[TESTING.md](https://github.com/CManson38/silo-plugin-sonarr-prefetch/blob/main/TESTING.md) records earlier live Linux amd64 checks against Silo build `44b7930e` and Sonarr `4.0.20.3014`, including an installed plugin receiving a real session event and requesting monitoring/searches for The Simpsons S02E07–E08. A separate five-episode processor check also exercised the real Sonarr APIs.

[The v0.2.1 release workflow](https://github.com/CManson38/silo-plugin-sonarr-prefetch/actions/runs/39744652700) passed, including race tests, platform builds, metadata checks, publication, and the standalone catalog update. All twelve published assets were downloaded and their digests and package contents verified. The token-absent notification path passed; the token-present dispatch has not been exercised against the community catalog.

The configurable playback threshold is covered by automated tests, including stopping below it and crossing it on a later event. It has not been live-tested on the operator's server. Version 0.2.1 has not been installed there. ARM and macOS builds are cross-compiled; runtime execution on those platforms and completed imports have not been verified. A community reviewer should exercise connection, setup, and threshold-triggered searches using the published release.

### Ownership

The repository is public but currently belongs to my account. I understand approval requires it to belong to Silo-Community. Could you confirm the transfer process for this repository? I saw the request to transfer Scrob through `Quick104` in issue #11 and can follow the process you specify. After transfer, the manifest URLs, publisher, Go module/import paths, support links, and custom catalog URL will need updating to the new home.

### AI disclosure

- Harness/tools: OpenAI Codex.
- Model for this submission preparation: GPT-6; earlier model identifiers were not recorded.
- Involvement: AI-assisted implementation, tests, documentation, and release automation. Automated test evidence and earlier live checks are distinguished above. No independent adversarial review was performed as part of this preparation.

## After a transfer is arranged

1. Update `origin`, the Go module and its imports, manifest presentation URLs/publisher, README/support links, and private reporting configuration to the actual new repository. Keep `community.requests.prefetch` unchanged.
2. Update the compiled-plugin manifest test's expected source URL and regenerate the standalone catalog from published release assets. Publish a new patch release from the transferred source so embedded metadata agrees with its new home.
3. Confirm Actions can publish releases and update the standalone catalog under the destination's branch rules. If direct pushes to `main` are prohibited, arrange a PR-based catalog update with the maintainers.
4. Have a community maintainer exercise the published binary against a supported Silo version, including connection checks and the configured threshold. Record the review URL and date; the community maintainers add the approval registry entry.
5. Have an authorized maintainer set `SILO_PLUGINS_DISPATCH_TOKEN` with access to dispatch to `Silo-Community/silo-plugins`. Repository secrets are not transferred by this project tooling. Do not use the plugin's runtime API keys for this.
6. Ask the maintainer to run the catalog's manual update for the approved repository/tag, or publish the next release after approval. A dispatch cannot approve a plugin itself.
