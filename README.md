# TM-DNS

TM-DNS is planned as a local-first DNS-layer firewall for macOS machines that run for long periods of time and serve schools or similar independent organizations. The goal is a small, reliable daemon with a localhost dashboard for static records, realtime rule decisions, query timelines, host investigation, watched IPs/domains, reports, and one-click security list management.

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

## Current Recommendation

Build the first version as a single Go service:

- Go DNS resolver/proxy using `miekg/dns` or a thin internal DNS layer.
- Embedded HTTP UI and JSON API served from the same binary on localhost.
- SQLite in WAL mode for config, query summaries, audit events, and local timelines.
- In-memory hot path for DNS decisions: static records, host/group policy, compiled blocklists, cache, and watched targets.
- Async/batched event writer so DNS request latency does not wait on disk.
- Prometheus-compatible metrics endpoint plus an in-app system load page.
- macOS LaunchDaemon for boot-time DNS service plus a lightweight menu bar app for green/yellow/red status and dashboard access.

Blocky is a strong Go DNS/ad-blocking project and should inform features and configuration concepts, but depending on it directly would make UI-driven static records, per-IP timelines, and school-oriented workflows harder to own. See [docs/architecture.md](docs/architecture.md) for the reasoning.

## Readiness Assessment

The architecture is ready for an implementation spike, but not yet for a full production build. Before building the full service, we should prototype four high-risk areas:

1. DNS hot path: UDP/TCP serving, upstream forwarding, static records, sinkhole responses, and event queue latency.
2. SQLite tracking: batched writes, hourly rollups, retention pruning, WAL checkpointing, and dashboard query speed.
3. Block UX: one-click list enablement, allowlist override, realtime rule view, blocked-attempt dashboard, and block-page behavior.
4. macOS service model: LaunchDaemon install/restart, privileged port 53 binding, and menu bar status polling.

If those pass, the stack is appropriate for implementation.

## First Milestone

1. Go daemon with UDP/TCP DNS proxying, static A/AAAA/CNAME records, upstream forwarding, and simple cache.
2. SQLite schema for settings, static records, blocklist subscriptions, query events, and hourly rollups.
3. Localhost UI shell themed after the Emporia utility monitor with Dashboard, Realtime, Requests, Blocked, Hosts, Domains, Records, Rules, Lists, Policies, Reports, Timelines, Audit, Load, and Settings pages.
4. Blocklist updater with curated presets, source validation, atomic swaps, rollback, and single-click domain/site blocking.
5. Load dashboard tracking QPS, p95 latency, memory, CPU, database queue depth, dropped events, and list refresh status.
6. LaunchDaemon plus menu bar app status indicator.

## Prototype Run Path

The current prototype runs on non-privileged development ports:

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
