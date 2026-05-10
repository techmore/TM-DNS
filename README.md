# TM-DNS

TM-DNS is a local-first DNS-layer firewall for macOS machines that run for long periods of time and serve schools or similar independent organizations. It ships as a Go DNS daemon plus a native macOS app and web dashboard for static records, realtime rule decisions, query timelines, host investigation, reports, diagnostics, and one-click security list management.

## Product Direction

- Run 24/7 on macOS with LaunchDaemon support and low CPU, memory, and disk churn.
- Provide DNS over UDP and TCP on port 53, with optional modern resolver support later.
- Let users manage local static records from `http://127.0.0.1:<port>`.
- Track DNS requests, hosts, watched IPs, watched domains, block events, and timeline history.
- Provide firewall-style realtime views of requests, matching rules, policy decisions, and actions.
- Identify requesting hosts by IP first, with hostname, MAC address, DHCP/ARP-derived names, and manual labels where available.
- Let admins drill into a host to see request history, blocked attempts, timing patterns, rule matches, and reports.
- Offer one-click enablement for common public malware, phishing, ad/tracker, and school-appropriate blocklists.
- Show blocked access attempts clearly: who tried, what domain, which policy/list blocked it, when it happened, and what changed afterward.
- Provide an optional local block landing page for HTTP traffic while still safely sinkholing HTTPS-only blocked domains.
- Stay open source, easy to inspect, and deployable by mid to large independent schools without a complex server stack.
- Use the visual language from `techmore/Emporia-Vue3-Mac-Utility-Monitor`: dense local dashboard, olive/stone palette, compact cards, timeline-first operational views, and native macOS wrapper potential.

## Current Architecture

The current build uses:

- Go DNS resolver/proxy using `miekg/dns` or a thin internal DNS layer.
- Embedded HTTP UI and JSON API served from the same daemon.
- SQLite in WAL mode for config, query summaries, audit events, and local timelines.
- In-memory hot path for DNS decisions: static records, host/group policy, compiled blocklists, cache, and watched targets.
- Async/batched event writer so DNS request latency does not wait on disk.
- In-app load and diagnostics pages for CPU, memory, disk, database growth, event drops, sleep state, and service config.
- macOS LaunchDaemon for boot-time DNS service plus a native app for status, setup, blocklists, host investigation, updates, and dashboard access.

Blocky is a strong Go DNS/ad-blocking project and should inform features and configuration concepts, but depending on it directly would make UI-driven static records, per-IP timelines, and school-oriented workflows harder to own. See [docs/architecture.md](docs/architecture.md) for the reasoning.

## Readiness Assessment

TM-DNS is ready for controlled pilot testing. Before broad school-wide production use, validate these items on the target network:

1. DNS reachability from at least one wired client and one wireless client.
2. UniFi or DHCP DNS settings point clients at the TM-DNS Mac, not DHCP relay.
3. macOS sleep is disabled so the Mac behaves like an appliance.
4. `/api/diagnostics` has no warnings other than expected LAN admin exposure.
5. Event drops remain at zero during peak query volume.
6. Blocklist refreshes complete and false positives are reviewed before enabling aggressive lists.
7. The update path is tested during a maintenance window.

For an emergency stop:

```bash
sudo launchctl bootout system /Library/LaunchDaemons/com.techmore.tmdns.daemon.plist
```

For a service restart:

```bash
sudo launchctl kickstart -k system/com.techmore.tmdns.daemon
```

## First Milestone

1. Go daemon with UDP/TCP DNS proxying, static A/AAAA/CNAME records, upstream forwarding, and simple cache.
2. SQLite schema for settings, static records, blocklist subscriptions, query events, and hourly rollups.
3. Localhost UI shell themed after the Emporia utility monitor with Dashboard, Realtime, Requests, Blocked, Hosts, Domains, Records, Rules, Lists, Policies, Reports, Timelines, Audit, Load, and Settings pages.
4. Blocklist updater with curated presets, source validation, atomic swaps, rollback, and single-click domain/site blocking.
5. Load dashboard tracking QPS, p95 latency, memory, CPU, database queue depth, dropped events, and list refresh status.
6. LaunchDaemon plus menu bar app status indicator.

## Prototype Run Path

The development build runs on non-privileged development ports:

- DNS UDP/TCP: `127.0.0.1:1053`
- Web UI/API: `http://127.0.0.1:8080`
- Database: `tm-dns-dev.db`
- Debug log: `/tmp/tmdns-dev.log`

Build and run:

```bash
./scripts/dev-start.sh
```

Smoke test:

```bash
./scripts/smoke.sh
```

Expected defaults:

- `router.test` resolves to `192.168.1.1`
- `blocked.test` resolves to `0.0.0.0`
- normal domains forward to the configured upstream resolver

Detached local testing with `screen`:

```bash
./scripts/dev-start.sh
tail -f /tmp/tmdns-dev.log
./scripts/dev-stop.sh
```

Full local verification:

## LAN Run Path

For a live LAN resolver, the daemon can now choose the Mac's LAN IPv4 at startup. It asks macOS for hardware ports and prefers wired Ethernet/LAN/Thunderbolt devices over Wi-Fi when multiple active interfaces exist.

```bash
sudo TMDNS_DNS_ADDR=auto:53 \
  TMDNS_HTTP_ADDR=auto:8080 \
  TMDNS_DB_PATH="/Library/Application Support/TM-DNS/tm-dns.db" \
  TMDNS_LOG_LEVEL=info \
  "/Library/Application Support/TM-DNS/tmdns"
```

On a packaged install this same configuration is managed by `launchd` through `/Library/LaunchDaemons/com.techmore.tmdns.daemon.plist`, so admins normally should not run the daemon manually. `TMDNS_DNS_ADDR=auto:53` now binds DNS to `0.0.0.0:53` so loopback and LAN validation use the same listener. The app still detects the preferred wired LAN IP for setup instructions.

## macOS Installer Package

Build a local unsigned installer package:

```bash
./scripts/build-pkg.sh
```

By default the package is labeled with the build date and time:

```text
build/pkg/TM-DNS-1.0.YYYYMMDD.HHMM.pkg
```

Override it when needed:

```bash
TMDNS_VERSION=1.0.20260509.1815 ./scripts/build-pkg.sh
```

The package stages:

- `/Applications/TM-DNS.app`
- `/Library/Application Support/TM-DNS/tmdns`
- `/Library/LaunchDaemons/com.techmore.tmdns.daemon.plist`
- logs under `/Library/Logs/TM-DNS/`

The installer starts the LaunchDaemon after install. The daemon runs with `TMDNS_DNS_ADDR=auto:53`, `TMDNS_HTTP_ADDR=auto:8080`, and stores its database at `/Library/Application Support/TM-DNS/tm-dns.db`.

Install locally for testing:

```bash
sudo installer -pkg build/pkg/TM-DNS-1.0.YYYYMMDD.HHMM.pkg -target /
```

For GitHub-based updates, publish the signed and notarized `.pkg` as a GitHub Release asset. The safe updater path is: check the latest release metadata, compare it to the installed version shown by `/api/health`, download the `.pkg`, verify signature/notarization, then prompt the admin before running `installer`. Fully silent self-updates should wait until the package is consistently Developer ID signed and notarized.

Check service state:

```bash
sudo launchctl print system/com.techmore.tmdns.daemon
curl http://127.0.0.1:8080/api/health
curl http://127.0.0.1:8080/api/diagnostics
```

```bash
go test ./...
./scripts/dev-start.sh
./scripts/smoke.sh
```

## References

- Blocky docs: https://0xerr0r.github.io/blocky/
- `miekg/dns`: https://github.com/miekg/dns
- SQLite WAL: https://www.sqlite.org/wal.html
- Theme reference: https://github.com/techmore/Emporia-Vue3-Mac-Utility-Monitor
