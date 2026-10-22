# Prefetch for Silo

Automatically download upcoming TV episodes through Sonarr as you watch in Silo. Prefetch waits until an episode reaches 10% playback progress by default, then asks Sonarr to fetch missing episodes ahead of it.

You can configure how many episodes to fetch, when prefetching starts, which users and series qualify, and an optional second Sonarr instance for 4K content.

## Install

You need a Silo server with plugin support and a working Sonarr setup, including indexers and a download client. Add the series you want to prefetch to Sonarr first.

1. Download the appropriate ZIP from [GitHub Releases](https://github.com/CManson38/silo-plugin-sonarr-prefetch/releases), or build the packages below.
2. In Silo, open **Administration → Plugins → Install from a file**. Upload the archive for the machine running **Silo**, not your browser:
   - `prefetch-0.2.0-linux-amd64.zip`: typical Unraid / x86 Docker server.
   - `prefetch-0.2.0-linux-arm64.zip`: ARM Linux server.
   - `prefetch-0.2.0-darwin-arm64.zip`: Apple Silicon macOS server.
3. Open **Prefetch** and fill in its settings. Leave **Enable prefetch** off initially.
4. Save, then open **Prefetch status → Test connections**. All configured services must show **Connected**.
5. Enable prefetch and save. Play an episode from an existing Sonarr series. Review **Prefetch status** and Sonarr **Activity**.

Upload the ZIP directly; you do not need to extract it. Archive checksums are included with each release in `SHA256SUMS`. This plugin is available through manual installation and is not currently listed in the approved community catalog.

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

Prefetch responds to Silo's playback-session updates. Allow time for Silo to report playback progress before checking the result.

The plugin discovers its own Silo server automatically, so there is no Silo URL to configure. It uses Silo's v2 API and needs an administrator API key to read active sessions.

**Minimum playback progress (%)** controls when prefetching starts. The default is 10%; set it to 0 for immediate prefetch. Stopping before the threshold causes no prefetch. Progress is based on the current position in the episode, so resuming or seeking past the threshold qualifies. If Silo does not know the episode's duration, Prefetch waits until it can determine progress.

For each eligible session and each Sonarr instance:

1. Find an existing series by title or alternate title. Ambiguous matches are skipped, so Silo and Sonarr must use matching titles.
2. Apply user/profile and Sonarr tag filters.
3. Select the next N known episodes in season/episode order, continuing across seasons and skipping specials.
4. Enable series monitoring if needed, then monitor missing episodes individually. Preserve the rest of the series and existing season monitoring flags.
5. Search for aired missing episodes, excluding queued downloads and episodes searched in the last 15 minutes.

With **episodes ahead = 3**, playing S01E07 checks S01E08, S02E01, and S02E02 when those are the next known episodes. If S01E08 is present, only the other two need monitoring. Unaired episodes are monitored for Sonarr's RSS handling but are not searched early; episodes with no known air date are also not searched.

Movies, specials, paused sessions, and incomplete episode metadata are ignored. Prefetch never adds a new series or requests a whole-season search. It monitors known episodes within the window, without enabling automatic monitoring of unannounced seasons. Sonarr selects releases and sends downloads to its configured clients; a submitted search does not guarantee that a matching release exists or finishes downloading in time.

Successful session/instance work and recent searches are remembered for 15 minutes in process memory. Concurrent notifications are serialized. A failing Sonarr does not prevent the other from processing. Retries skip successful instance work. Hosts with durable plugin event delivery retry failed calls. After a restart or across multiple Silo API replicas, memory is independent: queue checks still apply, but searches can repeat before downloads appear in Sonarr's queue.

Only the discovered local Silo API and configured Sonarr endpoints receive requests. There is no telemetry. Redirects are refused. Status and errors omit API keys and upstream response bodies. Status and connection-check routes require a Silo administrator.

## Verify on your server

1. Save your settings and use **Prefetch status → Test connections**.
2. Enable prefetch. Choose a series that exists in Sonarr and has an aired missing episode within your configured window.
3. Play an episode in Silo using an allowed profile and reach the configured progress threshold.
4. Open **Prefetch status** to see the result. In Sonarr, the selected missing episodes should be monitored, and matching releases should appear in **Activity → Queue**.

Connection checks only read services; they do not request downloads. See [TESTING.md](TESTING.md) for automated and live verification details.

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

Licensed under the [MIT License](LICENSE).
