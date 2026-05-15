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
2. UniFi or DHCP DNS settings point clients at the TM-DNS Mac as DNS servers, not DHCP relay.
3. Two onsite DNS servers are deployed, advertised by DHCP, and heartbeating before production rollout.
4. macOS sleep is disabled so each Mac behaves like an appliance.
5. `/api/diagnostics` has no warnings other than expected LAN admin exposure.
6. Event drops remain at zero during peak query volume.
7. Blocklist refreshes complete and false positives are reviewed before enabling aggressive lists.
8. The update path is tested during a maintenance window.

## New User Quick Start

TM-DNS is infrastructure. Treat it like a firewall or DHCP server, not a casual desktop app. A single TM-DNS Mac can work for a pilot, but production school networks should use two onsite Macs so DNS keeps working if one Mac restarts, sleeps, updates, loses network, or fails.

Recommended first deployment:

1. Choose two wired Macs or Mac minis.
2. Give both Macs static IPs or DHCP reservations.
3. Install TM-DNS on both Macs using the signed/notarized `.pkg`.
4. Open `/Applications/TM-DNS.app` on each Mac and confirm the service is online.
5. Disable macOS sleep on both Macs.
6. Configure one node as `Primary` and the other as `Secondary` in Settings.
7. Enter the peer API URL and peer admin token on both nodes.
8. Run `Heartbeat` and confirm the overview no longer warns about missing redundant DNS.
9. Run `Push Sync` from the primary after rules, static records, block lists, or retention settings change.
10. In DHCP, advertise both TM-DNS IPs as DNS servers.

Do not set TM-DNS as a DHCP relay target. DHCP relay is not DNS. In UniFi, leave DHCP mode as DHCP Server and set TM-DNS under DNS server options for the network.

## Redundant DNS

For production, DHCP should hand out two DNS server IPs:

```text
DNS Server 1: primary TM-DNS Mac static IP
DNS Server 2: secondary TM-DNS Mac static IP
```

Recommended topology:

```text
Clients -> DHCP DNS option -> primary TM-DNS, secondary TM-DNS
Primary TM-DNS -> upstream DNS resolver
Secondary TM-DNS -> upstream DNS resolver
Primary TM-DNS -> HA Push Sync -> Secondary TM-DNS
```

Setup flow:

1. Install the same TM-DNS release on both Macs.
2. Give each Mac a wired static IP or DHCP reservation.
3. Open TM-DNS on the secondary Mac and go to Settings -> Onsite Secondary DNS.
4. Enter the primary API URL, this Mac's API URL, this Mac's admin token, then click `Request Join`.
5. Open TM-DNS on the primary Mac and go to Settings -> Onsite Secondary DNS.
6. Click `Refresh Pending Requests`, review the requesting node name and URL, then click `Accept`.
7. The primary stores the secondary as its peer and attempts to configure the secondary back to the primary.
8. On the primary, click `Heartbeat`. It should report healthy.
9. On the primary, click `Push Sync`.
10. In DHCP, advertise both Mac IPs as DNS servers. Do not use DHCP relay.
11. Renew DHCP on a test client and confirm queries appear in TM-DNS.

The admin token is stored on each Mac in the TM-DNS application support directory unless `TMDNS_ADMIN_TOKEN` is set. When configuring a peer, use the token from the other Mac. Keep HA traffic on a trusted management LAN or VLAN.

Pairing is approval-based. The secondary can request to join, but the primary must accept the pending request before HA settings are changed. Join requests include the secondary admin token so the primary can configure and sync to it; keep pairing on a trusted LAN, and prefer HTTPS when it is available.

The primary pushes policy to the secondary. The secondary receives:

- static records
- rules and rule enabled/disabled state
- block list preset enabled/disabled state
- custom block list sources
- retention settings

The secondary does not receive local query history, host observations, UniFi API keys, admin tokens, updater state, or machine-specific settings. Those remain local to each Mac.

The native Overview page and web Dashboard warn when TM-DNS cannot verify a redundant peer. The warning clears when secondary DNS is enabled, configured with a peer token, and the heartbeat is current.

Sync is currently operator-controlled from the UI. After changing rules, static records, blocklist presets, custom blocklist sources, or retention settings on the primary, click `Push Sync`. Query history and host observations are intentionally local to each node because each DNS server sees the client traffic that reaches it.

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

## LAN Run Path

For a live LAN resolver, the daemon can choose the Mac's LAN IPv4 at startup. It asks macOS for hardware ports and prefers wired Ethernet/LAN/Thunderbolt devices over Wi-Fi when multiple active interfaces exist.

```bash
sudo TMDNS_DNS_ADDR=auto:53 \
  TMDNS_HTTP_ADDR=auto:8080 \
  TMDNS_DB_PATH="/Library/Application Support/TM-DNS/tm-dns.db" \
  TMDNS_LOG_LEVEL=info \
  "/Library/Application Support/TM-DNS/tmdns"
```

On a packaged install this configuration is managed by `launchd` through `/Library/LaunchDaemons/com.techmore.tmdns.daemon.plist`, so admins normally should not run the daemon manually. `TMDNS_DNS_ADDR=auto:53` binds DNS to the selected LAN listener so loopback and LAN validation use the same service. The app detects the preferred wired LAN IP for setup instructions.

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

Validate DNS locally:

```bash
dig @127.0.0.1 example.com
dig @127.0.0.1 router.test
dig @127.0.0.1 blocked.test
```

Validate DNS from the LAN using the detected Mac IP shown in the app:

```bash
dig @<tm-dns-lan-ip> example.com
```

Validate that clients are actually using TM-DNS:

1. Point a test VLAN or a small DHCP scope at both TM-DNS IPs.
2. Renew DHCP on one wired and one wireless client.
3. Query a domain from each client.
4. Confirm the Realtime and Hosts pages show the client IPs and domains.

If the network loses DNS, revert DHCP DNS server settings to the router or a known-good resolver, then restart affected clients or renew DHCP leases.

Development verification:

```bash
go test ./...
xcodebuild -project xcode-TM-DNS/xcode-TM-DNS.xcodeproj -scheme xcode-TM-DNS -configuration Debug build
./scripts/dev-start.sh
./scripts/smoke.sh
```

## References

- Blocky docs: https://0xerr0r.github.io/blocky/
- `miekg/dns`: https://github.com/miekg/dns
- SQLite WAL: https://www.sqlite.org/wal.html
- Theme reference: https://github.com/techmore/Emporia-Vue3-Mac-Utility-Monitor
