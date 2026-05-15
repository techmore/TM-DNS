package store

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), t.TempDir()+"/test.db", slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedDefaults(context.Background()); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
	return st
}

func TestNormalizeName(t *testing.T) {
	tests := map[string]string{
		"Example.COM":  "example.com.",
		".Example.COM": "example.com.",
		"example.com.": "example.com.",
		"":             "",
	}
	for input, want := range tests {
		if got := NormalizeName(input); got != want {
			t.Fatalf("NormalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAddRuleIsIdempotentAndMatchesSuffix(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	first, err := st.AddRule(ctx, "example.org", "block", "first")
	if err != nil {
		t.Fatalf("add first rule: %v", err)
	}
	second, err := st.AddRule(ctx, "example.org.", "block", "second")
	if err != nil {
		t.Fatalf("add duplicate rule: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate rule created: first=%d second=%d", first.ID, second.ID)
	}
	rule, err := st.MatchRule(ctx, "sub.example.org")
	if err != nil {
		t.Fatalf("match rule: %v", err)
	}
	if rule == nil || rule.ID != first.ID {
		t.Fatalf("expected suffix match for rule %d, got %#v", first.ID, rule)
	}

	disabled, err := st.SetRuleEnabled(ctx, first.ID, false)
	if err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("expected rule to be disabled")
	}
	rule, err = st.MatchRule(ctx, "sub.example.org")
	if err != nil {
		t.Fatalf("match disabled rule: %v", err)
	}
	if rule != nil {
		t.Fatalf("disabled rule still matched: %#v", rule)
	}
}

func TestBlocklistPresetToggle(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	presets, err := st.BlocklistPresets(ctx)
	if err != nil {
		t.Fatalf("blocklist presets: %v", err)
	}
	if len(presets) == 0 {
		t.Fatal("expected seeded blocklist presets")
	}
	updated, err := st.SetBlocklistPresetEnabled(ctx, presets[0].ID, true)
	if err != nil {
		t.Fatalf("enable preset: %v", err)
	}
	if !updated.Enabled {
		t.Fatal("expected preset enabled")
	}
	updated, err = st.SetBlocklistPresetEnabled(ctx, presets[0].ID, false)
	if err != nil {
		t.Fatalf("disable preset: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected preset disabled")
	}
}

func TestBlocklistSourceAddAndToggle(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	source, err := st.AddBlocklistSource(ctx, "Custom GitHub List", "https://raw.githubusercontent.com/example/list/main/domains.txt", "domains")
	if err != nil {
		t.Fatalf("add blocklist source: %v", err)
	}
	if !source.Enabled {
		t.Fatal("new source should be enabled")
	}
	if source.Format != "domains" {
		t.Fatalf("format = %q, want domains", source.Format)
	}

	disabled, err := st.SetBlocklistSourceEnabled(ctx, source.ID, false)
	if err != nil {
		t.Fatalf("disable source: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("expected source disabled")
	}

	sources, err := st.BlocklistSources(ctx)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources len = %d, want 1", len(sources))
	}
}

func TestUpdateHostIdentity(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	hostID, label, err := st.EnsureHost(ctx, "192.0.2.44")
	if err != nil {
		t.Fatalf("ensure host: %v", err)
	}
	if label != "192.0.2.44" {
		t.Fatalf("label = %q, want source IP", label)
	}

	err = st.UpdateHostIdentity(ctx, hostID, HostIdentityUpdate{
		Hostname:   "Classroom-Mac.local.",
		MAC:        "d0:11:e5:b1:de:c0",
		Vendor:     "Apple",
		Confidence: "mdns",
	})
	if err != nil {
		t.Fatalf("update identity: %v", err)
	}
	host, err := st.Host(ctx, hostID)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	if host.Hostname != "classroom-mac.local" || host.MAC != "d0:11:e5:b1:de:c0" || host.Vendor != "Apple" {
		t.Fatalf("identity = hostname %q mac %q vendor %q", host.Hostname, host.MAC, host.Vendor)
	}
	if host.IdentityConfidence != "mdns" || host.IdentityLastChecked == "" {
		t.Fatalf("confidence = %q last_checked = %q", host.IdentityConfidence, host.IdentityLastChecked)
	}
}

func TestIdentityParsers(t *testing.T) {
	host, mac := parseARPOutput("? (192.168.222.8) at d0:11:e5:b1:de:c0 on en13 ifscope [ethernet]", "192.168.222.8")
	if host != "" || mac != "d0:11:e5:b1:de:c0" {
		t.Fatalf("parse arp unknown = host %q mac %q", host, mac)
	}
	host, mac = parseARPOutput("classroom.local (192.168.222.20) at 0:e0:4c:99:92:79 on en13 ifscope [ethernet]", "192.168.222.20")
	if host != "classroom.local" || mac != "00:e0:4c:99:92:79" {
		t.Fatalf("parse arp named = host %q mac %q", host, mac)
	}
	if got := reversePTRName("192.168.222.8"); got != "8.222.168.192.in-addr.arpa." {
		t.Fatalf("reversePTRName = %q", got)
	}
	if got := parseDNSSDPTR([]byte("12:00:00.000  Add     2  4  192.168.222.8.in-addr.arpa. PTR mac-mini.local.")); got != "mac-mini.local" {
		t.Fatalf("parseDNSSDPTR = %q", got)
	}
	if got := vendorFromMAC("d0:11:e5:b1:de:c0"); got != "Apple" {
		t.Fatalf("vendor = %q, want Apple", got)
	}
}

func TestUniFiSettingsAndImport(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	apiKey := "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != apiKey {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"ip":"192.168.222.25","hostname":"Library-iMac.local","mac":"d0:11:e5:b1:de:c0","oui":"Apple"}]}`))
	}))
	t.Cleanup(server.Close)

	saved, err := st.SaveUniFiSettings(ctx, UniFiSettings{Enabled: true, BaseURL: server.URL, Site: "default", APIKey: apiKey})
	if err != nil {
		t.Fatalf("save unifi settings: %v", err)
	}
	if saved.APIKey != "" || !saved.HasAPIKey {
		t.Fatalf("saved settings leaked key or missed key flag: %#v", saved)
	}
	var rawKey string
	if err := st.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'unifi.api_key'`).Scan(&rawKey); err != nil {
		t.Fatalf("raw key setting: %v", err)
	}
	if rawKey == apiKey || !strings.HasPrefix(rawKey, "enc:v1:") {
		t.Fatalf("api key was not encrypted at rest: %q", rawKey)
	}
	result, err := st.TestUniFi(ctx)
	if err != nil {
		t.Fatalf("test unifi: %v", err)
	}
	if result.Seen != 1 {
		t.Fatalf("test saw %d clients, want 1", result.Seen)
	}
	result, err = st.ImportUniFiClients(ctx)
	if err != nil {
		t.Fatalf("import unifi: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Updated)
	}
	hosts, err := st.Hosts(ctx)
	if err != nil {
		t.Fatalf("hosts: %v", err)
	}
	var found Host
	for _, host := range hosts {
		if host.SourceIP == "192.168.222.25" {
			found = host
		}
	}
	if found.Hostname != "library-imac.local" || found.MAC != "d0:11:e5:b1:de:c0" || found.IdentityConfidence != "unifi" {
		t.Fatalf("imported host = %#v", found)
	}
}

func TestHASettingsAndSyncPayload(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	saved, err := st.SaveHASettings(ctx, HASettings{
		Enabled:   true,
		Role:      "primary",
		PeerName:  "secondary",
		PeerURL:   "http://192.0.2.11:8080",
		PeerToken: "secret",
	})
	if err != nil {
		t.Fatalf("save HA settings: %v", err)
	}
	if !saved.Enabled || saved.Role != "primary" || saved.PeerURL != "http://192.0.2.11:8080" || !saved.HasPeerToken || saved.PeerToken != "" {
		t.Fatalf("unexpected saved HA settings: %+v", saved)
	}
	if err := st.UpsertStaticRecord(ctx, StaticRecord{Name: "ha.test", Type: "A", Value: "192.0.2.10", TTL: 60}); err != nil {
		t.Fatalf("upsert record: %v", err)
	}
	if _, err := st.AddRule(ctx, "blocked-ha.test", "block", "ha sync"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if _, err := st.SetBlocklistPresetEnabled(ctx, "hagezi-pro", true); err != nil {
		t.Fatalf("enable preset: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO blocklist_sources(name, url, format, enabled, created_at) VALUES('Synced Custom', 'https://example.com/list.txt', 'domains', 1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	payload, err := st.ExportHASyncPayload(ctx)
	if err != nil {
		t.Fatalf("export payload: %v", err)
	}

	secondary := testStore(t)
	if err := secondary.UpsertStaticRecord(ctx, StaticRecord{Name: "stale.test", Type: "A", Value: "192.0.2.200", TTL: 60}); err != nil {
		t.Fatalf("stale record: %v", err)
	}
	if _, err := secondary.AddRule(ctx, "stale-block.test", "block", "stale"); err != nil {
		t.Fatalf("stale rule: %v", err)
	}
	if _, err := secondary.db.ExecContext(ctx, `INSERT INTO blocklist_sources(name, url, format, enabled, created_at) VALUES('Stale Custom', 'https://example.com/stale.txt', 'domains', 1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("stale source: %v", err)
	}
	result, err := secondary.ApplyHASyncPayload(ctx, payload)
	if err != nil {
		t.Fatalf("apply payload: %v", err)
	}
	if result.StaticRecords == 0 || result.Rules == 0 || result.BlocklistPresets == 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if rule, err := secondary.MatchRule(ctx, "blocked-ha.test"); err != nil || rule == nil || !rule.Enabled {
		t.Fatalf("synced rule = %+v err=%v", rule, err)
	}
	records, err := secondary.StaticRecords(ctx)
	if err != nil {
		t.Fatalf("secondary records: %v", err)
	}
	found := false
	for _, record := range records {
		if record.Name == "ha.test." && record.Value == "192.0.2.10" {
			found = true
		}
	}
	if !found {
		t.Fatal("synced static record not found")
	}
	records, err = secondary.StaticRecords(ctx)
	if err != nil {
		t.Fatalf("secondary records after sync: %v", err)
	}
	for _, record := range records {
		if record.Name == "stale.test." {
			t.Fatal("stale static record was not removed by authoritative sync")
		}
	}
	if rule, err := secondary.MatchRule(ctx, "stale-block.test"); err != nil || rule != nil {
		t.Fatalf("stale rule still matched after sync: %+v err=%v", rule, err)
	}
	sources, err := secondary.BlocklistSources(ctx)
	if err != nil {
		t.Fatalf("secondary sources: %v", err)
	}
	for _, source := range sources {
		if source.URL == "https://example.com/stale.txt" {
			t.Fatal("stale blocklist source was not removed by authoritative sync")
		}
	}
	foundSource := false
	for _, source := range sources {
		if source.URL == "https://example.com/list.txt" {
			foundSource = true
		}
	}
	if !foundSource {
		t.Fatal("synced blocklist source not found")
	}
	payload.BlocklistSources = append(payload.BlocklistSources, BlocklistSource{Name: "Bad", URL: "file:///tmp/list.txt", Format: "domains", Enabled: true})
	if _, err := secondary.ApplyHASyncPayload(ctx, payload); err == nil {
		t.Fatal("expected invalid imported blocklist source URL to fail")
	}
}

func TestHAHealthDetectsStaleHeartbeat(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if _, err := st.SaveHASettings(ctx, HASettings{
		Enabled:   true,
		Role:      "secondary",
		PeerName:  "Primary",
		PeerURL:   "http://192.0.2.10:8080",
		PeerToken: "secret",
	}); err != nil {
		t.Fatalf("save HA settings: %v", err)
	}
	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339Nano)
	if err := st.setSetting(ctx, "ha.last_heartbeat", old); err != nil {
		t.Fatalf("set heartbeat: %v", err)
	}
	if err := st.setSetting(ctx, "ha.last_status", "ok"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	health, err := st.HAHealth(ctx)
	if err != nil {
		t.Fatalf("ha health: %v", err)
	}
	if !health.Enabled || !health.Configured || !health.Stale {
		t.Fatalf("unexpected health: %+v", health)
	}
}

func TestParseBlocklistDomainsAndMatch(t *testing.T) {
	input := `
# comment
0.0.0.0 bad.example
||tracker.example^
*.wild.example
not a domain
`
	domains, err := parseBlocklistDomains(strings.NewReader(input), "hosts")
	if err != nil {
		t.Fatalf("parse blocklist: %v", err)
	}
	for _, domain := range []string{"bad.example.", "tracker.example.", "wild.example."} {
		if _, ok := domains[domain]; !ok {
			t.Fatalf("missing parsed domain %s in %#v", domain, domains)
		}
	}

	st := testStore(t)
	ctx := context.Background()
	err = st.replaceBlocklistEntries(ctx, blocklistFetchSource{ID: "test", Type: "preset", Name: "Test List"}, domains)
	if err != nil {
		t.Fatalf("replace entries: %v", err)
	}
	match, err := st.MatchBlocklist(ctx, "sub.bad.example")
	if err != nil {
		t.Fatalf("match blocklist: %v", err)
	}
	if match == nil || match.SourceName != "Test List" {
		t.Fatalf("match = %#v, want Test List", match)
	}
}

func TestInsertQueryEventBuildsHostDetailAndReport(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	hostID, _, err := st.EnsureHost(ctx, "192.0.2.10")
	if err != nil {
		t.Fatalf("ensure host: %v", err)
	}
	rule, err := st.AddRule(ctx, "blocked.example", "block", "test")
	if err != nil {
		t.Fatalf("add rule: %v", err)
	}
	now := time.Now().UTC()
	if err := st.InsertQueryEvent(ctx, QueryEvent{
		Timestamp:     now,
		HostID:        hostID,
		SourceIP:      "192.0.2.10",
		QueryName:     "blocked.example.",
		QueryType:     "A",
		Action:        "blocked",
		MatchedRuleID: &rule.ID,
		MatchedSource: "rule:blocked.example.",
		ResponseCode:  "NOERROR",
		LatencyMS:     1,
		AnswerSummary: "sinkhole",
	}); err != nil {
		t.Fatalf("insert blocked event: %v", err)
	}
	if err := st.InsertQueryEvent(ctx, QueryEvent{
		Timestamp:     now,
		HostID:        hostID,
		SourceIP:      "192.0.2.10",
		QueryName:     "allowed.example.",
		QueryType:     "A",
		Action:        "allowed",
		ResponseCode:  "NOERROR",
		Upstream:      "1.1.1.1:53",
		LatencyMS:     5,
		AnswerSummary: "93.184.216.34",
	}); err != nil {
		t.Fatalf("insert allowed event: %v", err)
	}

	detail, err := st.HostDetail(ctx, hostID, 24)
	if err != nil {
		t.Fatalf("host detail: %v", err)
	}
	if detail.Host.QueryCount != 2 || detail.Host.BlockCount != 1 {
		t.Fatalf("host counters = queries %d blocks %d, want 2/1", detail.Host.QueryCount, detail.Host.BlockCount)
	}
	if len(detail.Recent) != 2 || len(detail.Blocked) != 1 {
		t.Fatalf("detail lengths recent=%d blocked=%d", len(detail.Recent), len(detail.Blocked))
	}
	if detail.TotalQueries != 2 || detail.TotalBlocked != 1 || detail.UniqueDomains != 2 {
		t.Fatalf("detail totals = queries %d blocks %d domains %d", detail.TotalQueries, detail.TotalBlocked, detail.UniqueDomains)
	}

	report, err := st.HostReport(ctx, hostID)
	if err != nil {
		t.Fatalf("host report: %v", err)
	}
	if report.TotalQueries != 2 || report.TotalBlocked != 1 || report.UniqueDomains != 2 {
		t.Fatalf("report totals = queries %d blocks %d domains %d", report.TotalQueries, report.TotalBlocked, report.UniqueDomains)
	}
	if len(report.RecommendedNotes) == 0 {
		t.Fatal("expected report notes")
	}
}

func TestDashboardTopDomainsIncludesLast48HourPercent(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	hostID, _, err := st.EnsureHost(ctx, "192.0.2.20")
	if err != nil {
		t.Fatalf("ensure host: %v", err)
	}
	now := time.Now().UTC()
	events := []QueryEvent{
		{Timestamp: now, HostID: hostID, SourceIP: "192.0.2.20", QueryName: "top.example.", QueryType: "A", Action: "allowed"},
		{Timestamp: now.Add(-time.Hour), HostID: hostID, SourceIP: "192.0.2.20", QueryName: "top.example.", QueryType: "A", Action: "allowed"},
		{Timestamp: now.Add(-2 * time.Hour), HostID: hostID, SourceIP: "192.0.2.20", QueryName: "other.example.", QueryType: "A", Action: "allowed"},
		{Timestamp: now.Add(-25 * time.Hour), HostID: hostID, SourceIP: "192.0.2.20", QueryName: "older.example.", QueryType: "A", Action: "allowed"},
		{Timestamp: now.Add(-49 * time.Hour), HostID: hostID, SourceIP: "192.0.2.20", QueryName: "old.example.", QueryType: "A", Action: "allowed"},
	}
	for _, event := range events {
		if err := st.InsertQueryEvent(ctx, event); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}

	dashboard, err := st.Dashboard(ctx, "test.db")
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if len(dashboard.TopDomains) == 0 {
		t.Fatal("expected top domains")
	}
	top := dashboard.TopDomains[0]
	if top.Key != "top.example." || top.Count != 2 {
		t.Fatalf("top domain = %s/%d, want top.example./2", top.Key, top.Count)
	}
	if top.Percent < 49.9 || top.Percent > 50.1 {
		t.Fatalf("top percent = %.2f, want about 50", top.Percent)
	}
	for _, row := range dashboard.TopDomains {
		if row.Key == "old.example." {
			t.Fatal("old event was included in rolling 48-hour top domains")
		}
	}
}
