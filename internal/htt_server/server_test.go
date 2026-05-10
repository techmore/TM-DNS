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
