package htt_server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/techmore/tm-dns/internal/config"
	"github.com/techmore/tm-dns/internal/dnsserver"
	"github.com/techmore/tm-dns/internal/store"
)

func TestAPIHostReportAndRuleCreation(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := store.Open(ctx, t.TempDir()+"/api.db", logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
	hostID, _, err := st.EnsureHost(ctx, "192.0.2.20")
	if err != nil {
		t.Fatalf("ensure host: %v", err)
	}
	if err := st.InsertQueryEvent(ctx, store.QueryEvent{
		Timestamp:     time.Now().UTC(),
		HostID:        hostID,
		SourceIP:      "192.0.2.20",
		QueryName:     "example.test.",
		QueryType:     "A",
		Action:        "allowed",
		ResponseCode:  "NOERROR",
		LatencyMS:     1,
		AnswerSummary: "192.0.2.1",
	}); err != nil {
		t.Fatalf("insert query: %v", err)
	}

	cfg := config.Config{DNSAddr: "127.0.0.1:1053", HTTPAddr: "127.0.0.1:8080", DBPath: "test.db", Upstream: "1.1.1.1:53", EventQueueCap: 10}
	cfg.AdminToken = "test-token"
	dns := dnsserver.New(cfg, st, logger)
	srv := New(cfg, st, dns, logger)

	req := httptest.NewRequest(http.MethodPost, "/api/rules/block", strings.NewReader(`{"target":"newblock.test","note":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rule create status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/reports/host/1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host report status = %d body=%s", rec.Code, rec.Body.String())
	}
	var report store.HostReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.TotalQueries != 1 {
		t.Fatalf("report total queries = %d, want 1", report.TotalQueries)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", rec.Code, rec.Body.String())
	}
	var dashboard map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&dashboard); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if _, ok := dashboard["system"]; !ok {
		t.Fatal("dashboard missing system stats")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d body=%s", rec.Code, rec.Body.String())
	}
	var diagnostics map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&diagnostics); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if _, ok := diagnostics["warnings"]; !ok {
		t.Fatal("diagnostics missing warnings")
	}
	if _, ok := diagnostics["ha"]; !ok {
		t.Fatal("diagnostics missing ha health")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/blocklist-presets", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("blocklist presets status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/blocklist-sources", strings.NewReader(`{"name":"Custom","url":"https://raw.githubusercontent.com/example/list/main/domains.txt","format":"domains"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("blocklist source create status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/rules/1", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rule toggle status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/blocklist-presets/hagezi-pro", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preset toggle status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/ha/settings", strings.NewReader(`{"enabled":true,"role":"primary","peer_name":"Secondary","peer_url":"http://192.0.2.11:8080","peer_token":"peer-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ha settings status = %d body=%s", rec.Code, rec.Body.String())
	}
	var haSettings store.HASettings
	if err := json.NewDecoder(rec.Body).Decode(&haSettings); err != nil {
		t.Fatalf("decode ha settings: %v", err)
	}
	if !haSettings.Enabled || !haSettings.HasPeerToken || haSettings.PeerToken != "" {
		t.Fatalf("unexpected ha settings: %+v", haSettings)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/ha/settings", strings.NewReader(`{"enabled":true,"role":"secondary","peer_name":"Primary","peer_url":"http://192.0.2.10:8080","peer_token":"peer-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("secondary ha settings status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/ha/import", strings.NewReader(`{"static_records":[{"name":"peer.test","type":"A","value":"192.0.2.99","ttl":60}],"rules":[{"target":"peer-block.test","action":"block","enabled":true,"note":"peer"}],"blocklist_presets":[],"blocklist_sources":[],"retention_settings":{"days":30}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ha import status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHAHeartbeatAndSyncRequirePeerAuthAndReceiverRole(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	secondaryStore, err := store.Open(ctx, t.TempDir()+"/secondary.db", logger)
	if err != nil {
		t.Fatalf("open secondary store: %v", err)
	}
	t.Cleanup(func() { _ = secondaryStore.Close() })
	if err := secondaryStore.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed secondary: %v", err)
	}
	secondaryCfg := config.Config{DNSAddr: "127.0.0.1:1053", HTTPAddr: "127.0.0.1:8080", DBPath: "secondary.db", Upstream: "1.1.1.1:53", EventQueueCap: 10, AdminToken: "shared-secret"}
	secondarySrv := New(secondaryCfg, secondaryStore, dnsserver.New(secondaryCfg, secondaryStore, logger), logger)
	secondaryHTTP := httptest.NewServer(secondarySrv.server.Handler)
	t.Cleanup(secondaryHTTP.Close)

	if _, err := secondaryStore.SaveHASettings(ctx, store.HASettings{Enabled: true, Role: "secondary", PeerName: "Primary", PeerURL: "http://primary.invalid:8080", PeerToken: "shared-secret"}); err != nil {
		t.Fatalf("save secondary ha: %v", err)
	}

	primaryStore, err := store.Open(ctx, t.TempDir()+"/primary.db", logger)
	if err != nil {
		t.Fatalf("open primary store: %v", err)
	}
	t.Cleanup(func() { _ = primaryStore.Close() })
	if err := primaryStore.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed primary: %v", err)
	}
	if err := primaryStore.UpsertStaticRecord(ctx, store.StaticRecord{Name: "sync.test", Type: "A", Value: "192.0.2.44", TTL: 60}); err != nil {
		t.Fatalf("primary record: %v", err)
	}
	if _, err := primaryStore.AddRule(ctx, "deny-sync.test", "block", "ha"); err != nil {
		t.Fatalf("primary rule: %v", err)
	}

	if _, err := primaryStore.SaveHASettings(ctx, store.HASettings{Enabled: true, Role: "primary", PeerName: "Secondary", PeerURL: secondaryHTTP.URL, PeerToken: "wrong-secret"}); err != nil {
		t.Fatalf("save primary wrong ha: %v", err)
	}
	status, err := primaryStore.TestHAPeer(ctx)
	if err != nil {
		t.Fatalf("test wrong peer: %v", err)
	}
	if status.Status != "error" {
		t.Fatalf("wrong token heartbeat status = %+v, want error", status)
	}

	if _, err := primaryStore.SaveHASettings(ctx, store.HASettings{Enabled: true, Role: "primary", PeerName: "Secondary", PeerURL: secondaryHTTP.URL, PeerToken: "shared-secret"}); err != nil {
		t.Fatalf("save primary ha: %v", err)
	}
	status, err = primaryStore.TestHAPeer(ctx)
	if err != nil {
		t.Fatalf("test peer: %v", err)
	}
	if status.Status != "ok" || status.PeerRole != "secondary" {
		t.Fatalf("heartbeat status = %+v, want ok secondary", status)
	}
	result, err := primaryStore.PushHASync(ctx)
	if err != nil {
		t.Fatalf("push sync: %v", err)
	}
	if result.Status != "ok" || result.StaticRecords == 0 || result.Rules == 0 {
		t.Fatalf("sync result = %+v", result)
	}
	if rule, err := secondaryStore.MatchRule(ctx, "deny-sync.test"); err != nil || rule == nil || !rule.Enabled {
		t.Fatalf("secondary synced rule = %+v err=%v", rule, err)
	}
}

func TestAPIRequiresAuth(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := store.Open(ctx, t.TempDir()+"/auth.db", logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{DNSAddr: "127.0.0.1:1053", HTTPAddr: "127.0.0.1:8080", DBPath: "test.db", Upstream: "1.1.1.1:53", EventQueueCap: 10, AdminToken: "secret"}
	srv := New(cfg, st, dnsserver.New(cfg, st, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard without auth status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"token":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHARequestJoinRejectsPublicTarget(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := store.Open(ctx, t.TempDir()+"/join.db", logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{DNSAddr: "127.0.0.1:1053", HTTPAddr: "127.0.0.1:8080", DBPath: "test.db", Upstream: "1.1.1.1:53", EventQueueCap: 10, AdminToken: "secret"}
	srv := New(cfg, st, dnsserver.New(cfg, st, logger), logger)

	req := httptest.NewRequest(http.MethodPost, "/api/ha/request-join", strings.NewReader(`{"primary_url":"http://8.8.8.8:8080"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("request join to public target unexpectedly succeeded: %s", rec.Body.String())
	}
}

func TestAPILoopbackBypassesAuth(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := store.Open(ctx, t.TempDir()+"/loopback.db", logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{DNSAddr: "127.0.0.1:1053", HTTPAddr: "127.0.0.1:8080", DBPath: "test.db", Upstream: "1.1.1.1:53", EventQueueCap: 10, AdminToken: "secret"}
	srv := New(cfg, st, dnsserver.New(cfg, st, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	req.RemoteAddr = "127.0.0.1:49152"
	rec := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback dashboard status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.RemoteAddr = "127.0.0.1:49152"
	rec = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback auth status = %d body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode auth status: %v", err)
	}
	if !status.Authenticated {
		t.Fatal("loopback auth status should be authenticated")
	}
}

func TestAPILocalInterfaceBypassesAuth(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatalf("interface addrs: %v", err)
	}
	var localIP net.IP
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			localIP = ip4
			break
		}
	}
	if localIP == nil {
		t.Skip("no non-loopback local IPv4 address")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	req.RemoteAddr = net.JoinHostPort(localIP.String(), "49152")
	if !isLoopbackRequest(req) {
		t.Fatalf("expected %s to be treated as local", req.RemoteAddr)
	}
}
