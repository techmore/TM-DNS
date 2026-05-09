package store

import (
	"context"
	"log/slog"
	"os"
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

	detail, err := st.HostDetail(ctx, hostID)
	if err != nil {
		t.Fatalf("host detail: %v", err)
	}
	if detail.Host.QueryCount != 2 || detail.Host.BlockCount != 1 {
		t.Fatalf("host counters = queries %d blocks %d, want 2/1", detail.Host.QueryCount, detail.Host.BlockCount)
	}
	if len(detail.Recent) != 2 || len(detail.Blocked) != 1 {
		t.Fatalf("detail lengths recent=%d blocked=%d", len(detail.Recent), len(detail.Blocked))
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
