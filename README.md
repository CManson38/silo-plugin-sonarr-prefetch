# Prefetch for Silo

Download upcoming TV episodes through Sonarr when someone starts watching in Silo. This native Silo plugin replaces the separate Prefetcharr polling service for this use case.

## Install

1. Download the appropriate ZIP from [GitHub Releases](https://github.com/CManson38/silo-plugin-sonarr-prefetch/releases), or build the packages below.
2. In Silo, open **Administration → Plugins → Install from a file**. Upload the archive for the machine running **Silo**, not your browser:
   - `prefetch-0.2.0-linux-amd64.zip`: typical Unraid / x86 Docker server.
   - `prefetch-0.2.0-linux-arm64.zip`: ARM Linux server.
   - `prefetch-0.2.0-darwin-arm64.zip`: Apple Silicon macOS server.
3. Open **Prefetch** and fill in its settings. Leave **Enable prefetch** off initially.
4. Save, then open **Prefetch status → Test connections**. All configured services must show **Connected**.
5. Enable prefetch and save. Play an episode from an existing Sonarr series. Review **Prefetch status** and Sonarr **Activity**.

Each ZIP contains `plugin` and `manifest.json` at its root, with the SHA-256 of that platform's binary. You can also upload the unpacked `plugin` binary; it reports its own manifest and checksum. Manual installation does not require community catalog approval. This plugin is not currently catalog-approved.

## Configure

| Setting | Value |
| --- | --- |
| Silo admin API key | Create an API key owned by an **administrator** and leave its **scopes empty (unrestricted)**. Prefetch reads `GET /api/v2/admin/sessions`; Silo currently has no restricted scope granting that endpoint. `admin:sessions:summary:read` is insufficient. |
| Sonarr URL / API key | Base URL, usually `http://sonarr:8989`, and **Settings → General → Security → API Key**. Reverse proxy base paths are supported. |
| 4K Sonarr URL / API key | Optional second instance. Supply both or leave both empty. Both instances prefetch wherever the series exists, regardless of the playing file's resolution. |
| Episodes ahead | 1–100; default 2. Counts the next N episodes, including those already downloaded. |
| Minimum playback progress (%) | 0–100; default 10. Wait until the playing episode reaches 10% before monitoring or searching upcoming episodes. Set 0 for immediate prefetch. |
| Include users / profiles | Empty allows everyone. Otherwise, an account ID, username, profile ID, or profile name must match. |
| Exclude users / profiles | Any matching account/profile ID or name skips that session. Exclusion wins over inclusion. |
| Include Sonarr tags | Empty allows all series. Otherwise, the series must have at least one matching **tag label**. |
| Exclude Sonarr tags | A series with any matching tag label is skipped. Exclusion wins over inclusion. |

Lists accept commas or one entry per line. Matching ignores case and surrounding whitespace. Prefer IDs for users/profiles with duplicate names. An account-level exclusion excludes every profile on that account. Tags resolve separately in each Sonarr instance. A failed tag lookup stops processing that instance instead of bypassing its filters.

URLs must be absolute HTTP(S) URLs without embedded credentials, query strings, or fragments. API keys are secret fields in Silo's settings form and travel in headers. The Silo container must have network/DNS access to Sonarr; `localhost` refers to that container. TLS certificates must be trusted by the server.

## Behavior

The plugin subscribes to `playback_sessions_changed` and `sessions.replaced` through `event_consumer.v1`. Each notification triggers a paginated read of active sessions from Silo's native API. It has no polling timer. Processing follows Silo's session reconciliation rather than happening synchronously inside the Play request.

Prefetch always asks `RuntimeHost.GetHostInfo` for its own Silo server's `internal_base_url` when processing events and testing connections. There is no URL setting. It still needs the administrator API key to read sessions. Connection checks report an error if the host connection is unavailable or the Silo build cannot report its internal API URL.

When **Minimum playback progress (%)** is above zero, session updates are ignored until `position_seconds / file_duration × 100` reaches the configured percentage. No Sonarr requests or monitoring changes happen below that threshold, and stopping before reaching it causes no prefetch. The next qualifying session update triggers prefetch. An unknown or zero duration waits for usable metadata. This measures playback position, not accumulated viewing time: resuming or seeking past the threshold qualifies. Omitted settings default to 10%; explicitly saved values, including 0, are preserved on upgrade.

For each eligible session and each Sonarr instance:

1. Find an existing series by title or alternate title. Ambiguous matches are skipped. The Silo sessions endpoint does not supply TVDB IDs, so titles must match.
2. Apply user/profile and Sonarr tag filters.
3. Select the next N known episodes in season/episode order, continuing across seasons and skipping specials.
4. Enable series monitoring if needed, then monitor missing episodes individually. Preserve the rest of the series and existing season monitoring flags.
5. Submit `EpisodeSearch` for aired missing episodes, excluding queued downloads and episodes searched in the last 15 minutes.

With **episodes ahead = 3**, playing S01E07 checks S01E08, S02E01, and S02E02 when those are the next known episodes. If S01E08 is present, only the other two need monitoring. Unaired episodes are monitored for Sonarr's RSS handling but are not searched early; episodes with no known air date are also not searched.

Movies, specials, paused sessions, and incomplete episode metadata are ignored. Prefetch never adds a new series or requests a whole-season search. It monitors known episodes within the window, without enabling automatic monitoring of unannounced seasons. Sonarr selects releases and sends downloads to its configured clients; a submitted search does not guarantee that a matching release exists or finishes downloading in time.

Successful session/instance work and recent searches are remembered for 15 minutes in process memory. Concurrent notifications are serialized. A failing Sonarr does not prevent the other from processing. Retries skip successful instance work. Hosts with durable plugin event delivery retry failed calls. After a restart or across multiple Silo API replicas, memory is independent: queue checks still apply, but searches can repeat before downloads appear in Sonarr's queue.

Only the discovered local Silo API and configured Sonarr endpoints receive requests. There is no telemetry. Redirects are refused. Status and errors omit API keys and upstream response bodies. Status and connection-check routes require a Silo administrator.

## Verify on your server

1. Leave Prefetch disabled and test connections.
2. Pick an allowed series with an aired missing episode within the next N positions. Note its monitoring flags.
3. Enable Prefetch and play the preceding episode using an allowed profile. If a progress threshold is set, wait until that percentage is reached. Allow Silo's session reconciliation to publish its event.
4. Confirm that Prefetch status reports the missing episode IDs. In Sonarr, confirm the individual monitoring flags and `EpisodeSearch` command in logs/command activity. A matching release should appear in **Activity → Queue**.
5. Reconcile/replay the same session and confirm another search is suppressed. With a nonzero progress threshold, starting then stopping below it should cause no monitoring/search. Check excluded users and series tags; neither should trigger monitoring/search.
6. After verification, stop the old Prefetcharr service so both services do not request the same episodes.

Connection checks only read services; they do not request downloads. Automated tests use HTTP fixtures plus a real compiled plugin process. See [TESTING.md](TESTING.md) for validation evidence and limits. Record live verification separately.

If status stays **Waiting for playback**, confirm your Silo build supports plugin event consumers and publishes these session events. A 401/403 means the API key/account permissions need checking; use an administrator-owned key with empty scopes. A discovery failure means the host broker or internal URL information is unavailable. A Silo 404 can indicate a build predating the native v2 endpoint. **Series not found** requires aligning Silo's title with a Sonarr title/alternate title.

## Build and test

Requires Go 1.26 or newer and Python 3 for packaging. The SDK is pinned to `github.com/Silo-Server/silo-plugin-sdk v0.28.0`. No local SDK replacement is needed.

```sh
go test ./...
go vet ./...
go build -o plugin .
./plugin manifest
python3 scripts/build.py
```

On Windows use `go build -o plugin.exe .`, `.\plugin.exe manifest`, and `python scripts/build.py`. The repository-local compiler, if present, is `.tools/go/bin/go.exe`. Packaging builds static binaries (`CGO_ENABLED=0`) for all three platforms into `dist/`, with uploadable ZIPs and `SHA256SUMS`.

CI runs race tests, vet, manifest introspection, and all platform builds, then uploads workflow artifacts. Pushing a version tag matching `manifest.json` (for example `v0.2.0`) publishes a GitHub release with all three platform ZIPs and `SHA256SUMS` after those checks pass.

## References and license

- [Silo SDK](https://github.com/Silo-Server/silo-plugin-sdk): runtime, configuration, and plugin contracts.
- [Silo event delivery](https://github.com/Silo-Server/silo-server/blob/main/docs/architecture/plugin-event-delivery.md): events, retries, and limits.
- [Scrob example](https://github.com/Joloxx9/silo-plugin-watchprovider-scrob): uses per-profile watch-provider calls; Prefetch uses global events and the admin sessions API.
- [Community catalog](https://github.com/Silo-Community/silo-plugins): approval and distribution.

MIT. This is a new Go implementation informed by the local [Prefetcharr](https://github.com/p-hueber/prefetcharr) project (Paul Hüber, MIT OR Apache-2.0). No Rust source or Scrob implementation is included.
