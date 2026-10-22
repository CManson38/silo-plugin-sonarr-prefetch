# Validation evidence

Validated on 22 October 2026. The operator installed the plugin on the live Silo server after the initial package checks; real playback verification is recorded below.

The automatic URL discovery update passed `go test -count=1 ./...`. The compiled plugin test now binds a real SDK host broker, serves `GetHostInfo`, configures the plugin without any URL setting, and verifies both playback handling and connection checks use the reported internal API URL. Unit tests verify discovery failures, caller cancellation, host URL changes, and ignoring an old saved URL. The manifest test confirms there is no URL field. The API key field description has since been restored to its original short text. These automated discovery checks use a simulated host; discovery was subsequently verified in the live installed plugin as recorded below.

## Automated checks

Catalog tests use Silo's standard JSON decoding into the SDK's manifest types, validate the published `index.json`, check release URLs and per-platform checksum formats, and reject missing or empty binary inputs. The catalog generator serializes protobuf enum values numerically for compatibility with Silo's catalog reader. The initial catalog's hashes were computed from the verified binaries inside the existing v0.2.0 release ZIPs.

Version 0.2.0 adds a configurable playback-progress threshold, defaulting to 10%. The compiled plugin test omits the threshold setting, sends sessions at 9.9% progress, removes the session to simulate an early stop, restores it below threshold, then advances it to exactly 10%. No search occurs until that boundary; repeated progress then causes no duplicate search. Additional tests cover missing/zero duration, invalid positions, resumed playback, configuration bounds, defaulting to 10%, preserving an explicit 0%, and ensuring below-threshold sessions cause neither monitoring nor search. This threshold update has not been installed or live-tested on the operator's server.

The initial `go test -cover ./...` passed using Go 1.27.3 on Windows amd64, with 90.1% statement coverage in `prefetch` before the discovery update. The executable entry point is exercised by a separately built process, so its in-process coverage reports 0%.

`go vet ./...` passed. Go files were formatted with `gofmt`. All three release targets built with CGO disabled. Their ZIP layout, binary SHA-256, executable permissions, and ELF/Mach-O architecture headers were checked.

| Behavior | Evidence |
| --- | --- |
| Native Silo plugin transport | Build the actual executable; introspect and validate its manifest/checksum; start it using the SDK's HashiCorp gRPC handshake; bind the host broker for automatic URL discovery; invoke Runtime.Configure, EventConsumer.HandleEvent, and HttpRoutes.Handle. |
| Fresh installation | Configure with no saved values and render diagnostics without preventing startup. |
| Event-driven prefetch | Send session-change notifications through the compiled plugin to HTTP fixtures and verify exactly one search for a repeated session. |
| Playback progress | Verify early-stop suppression, threshold crossing on later events, boundary values, unavailable duration, and no monitoring/search before eligibility. |
| Two Sonarr instances | Process both; fail the second search; retry without repeating the first instance's successful work. |
| Episode window | Continue across seasons; downloaded episodes still count toward N; exclude specials and the currently playing episode. |
| Monitoring and search | Monitor only missing episodes, preserve other series/season settings, and search only known aired episodes. |
| User/profile filters | Include by account ID/name or profile ID/name; exclusions override inclusions. |
| Tag filters | Resolve labels per Sonarr; include/exclude precedence; no mutation when tag lookup fails. |
| Series matching | Match alternate titles; skip missing and ambiguous matches. |
| Duplicate handling | Suppress queued/recent searches and concurrent duplicate notifications; failed searches remain retryable. |
| Silo sessions | Bearer authentication, reverse proxy base paths, pagination, cursor escaping, repeated-cursor protection, and malformed response rejection. |
| Configuration | Validate episode bounds, complete optional 4K settings, and absolute credential-free HTTP(S) URLs. |
| HTTP behavior | API keys in headers; refuse redirects; omit secrets from status/errors; honor RPC cancellation. |

The race detector cannot run in the local Windows environment because it has no C compiler. The GitHub Actions workflow runs `go test -race ./...` on Linux; release publication depends on that job passing.

## Live service checks

Read-only checks succeeded against Silo build `44b7930e` and both Sonarr instances running `4.0.20.3014`. Silo returned its native paginated sessions shape. There were no active sessions at the time of the check. The requested account/profile existed. Both Sonarr status and queue endpoints accepted the plugin's API requests.

The operator authorized The Simpsons S02E01 as the test input. The same `Processor.Process` implementation used by the plugin was run against the real Sonarr APIs with a simulated playback session for that episode and a five-episode window. Credentials were read from the existing configuration share and kept in memory only; none were written into this repository or its packages.

Observed results:

- Standard Sonarr contained S02E01 on disk; S02E02–E06 were missing and unmonitored before the test.
- Episode IDs 97056–97060 became monitored. S02E07 stayed unmonitored, confirming the window boundary.
- Sonarr accepted `EpisodeSearch` command 132840860 and completed it.
- After the search, four of the selected episodes appeared in the download queue with healthy tracking status: one downloading and three queued. This verifies download submission, not completed imports for every requested episode.
- Repeating the same session through the processor produced no second search.
- The 4K instance did not contain The Simpsons and correctly skipped it.

## Installed plugin and real playback

After the operator installed and configured the Linux amd64 package, real playback of The Simpsons S02E06 was visible in Silo under the requested profile. Connection checks passed, but the separate **Enable prefetch** processing setting was off despite the installation itself being enabled. That processing flag was enabled without changing other saved settings.

The installed plugin received a real session event at 2026-10-22T01:00:11Z and automatically discovered its own Silo API. With the operator's two-episode window, it monitored and searched S02E07–E08 (IDs 97061–97062). Sonarr completed `EpisodeSearch` command 132840933 at 01:00:18Z. E07 was downloading, E08 was queued, both had healthy tracking status, and E09 remained unmonitored. This verifies the real Silo event → installed plugin → monitoring → search → download queue path on Linux amd64.

Linux arm64 and macOS arm64 remain cross-compiled artifacts whose runtime execution has not been tested. Completed download imports were not checked.
