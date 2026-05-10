package htt_server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/techmore/tm-dns/internal/config"
	"github.com/techmore/tm-dns/internal/dnsserver"
	"github.com/techmore/tm-dns/internal/store"
	"github.com/techmore/tm-dns/internal/systemstats"
	"github.com/techmore/tm-dns/internal/version"
)

type Server struct {
	cfg        config.Config
	store      *store.Store
	dns        *dnsserver.Server
	logger     *slog.Logger
	server     *http.Server
	mux        *http.ServeMux
	stats      *systemstats.Sampler
	statMu     sync.Mutex
	statAt     time.Time
	stat       systemstats.Stats
	adminToken string
}

func New(cfg config.Config, st *store.Store, dns *dnsserver.Server, logger *slog.Logger) *Server {
	token, err := resolveAdminToken(cfg)
	if err != nil {
		logger.Error("admin token setup failed", "error", err)
	}
	s := &Server{cfg: cfg, store: st, dns: dns, logger: logger, mux: http.NewServeMux(), stats: systemstats.NewSampler(cfg.DBPath), adminToken: token}
	s.routes()
	s.server = &http.Server{Addr: cfg.HTTPAddr, Handler: logMiddleware(logger, s.authMiddleware(s.mux))}
	return s
}

func resolveAdminToken(cfg config.Config) (string, error) {
	if strings.TrimSpace(cfg.AdminToken) != "" {
		return strings.TrimSpace(cfg.AdminToken), nil
	}
	path := cfg.AdminTokenPath
	if path == "" {
		path = filepath.Join(filepath.Dir(cfg.DBPath), "admin-token.txt")
	}
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), 0600); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0600)
	return token, nil
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.Shutdown(context.Background())
	}()
	s.logger.Info("http listener starting", "addr", s.cfg.HTTPAddr)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.index)
	s.mux.HandleFunc("/api/auth/status", s.authStatus)
	s.mux.HandleFunc("/api/auth/login", s.authLogin)
	s.mux.HandleFunc("/api/auth/logout", s.authLogout)
	s.mux.HandleFunc("/api/health", s.health)
	s.mux.HandleFunc("/api/diagnostics", s.diagnostics)
	s.mux.HandleFunc("/api/dashboard", s.dashboard)
	s.mux.HandleFunc("/api/realtime", s.realtime)
	s.mux.HandleFunc("/api/blocked", s.blocked)
	s.mux.HandleFunc("/api/hosts", s.hosts)
	s.mux.HandleFunc("/api/hosts/", s.hostDetail)
	s.mux.HandleFunc("/api/rules", s.rules)
	s.mux.HandleFunc("/api/rules/", s.ruleDetail)
	s.mux.HandleFunc("/api/rules/block", s.ruleAction("block"))
	s.mux.HandleFunc("/api/rules/allow", s.ruleAction("allow"))
	s.mux.HandleFunc("/api/blocklist-presets", s.blocklistPresets)
	s.mux.HandleFunc("/api/blocklist-presets/", s.blocklistPresetDetail)
	s.mux.HandleFunc("/api/blocklist-sources", s.blocklistSources)
	s.mux.HandleFunc("/api/blocklist-sources/", s.blocklistSourceDetail)
	s.mux.HandleFunc("/api/blocklists/refresh", s.blocklistsRefresh)
	s.mux.HandleFunc("/api/records", s.records)
	s.mux.HandleFunc("/api/reports/host/", s.hostReport)
	s.mux.HandleFunc("/api/audit", s.audit)
	s.mux.HandleFunc("/api/settings/unifi", s.unifiSettings)
	s.mux.HandleFunc("/api/settings/unifi/test", s.unifiTest)
	s.mux.HandleFunc("/api/settings/unifi/import", s.unifiImport)
	s.mux.HandleFunc("/api/settings/retention", s.retentionSettings)
	s.mux.HandleFunc("/api/settings/retention/purge", s.retentionPurge)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, cookieAuth := s.authorized(r)
		if s.isPublicPath(r.URL.Path) || isLoopbackRequest(r) || ok {
			if ok && cookieAuth && !sameOriginUnsafeRequest(r) {
				http.Error(w, "invalid request origin", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) isPublicPath(path string) bool {
	return path == "/" || path == "/api/health" || path == "/api/auth/status" || path == "/api/auth/login"
}

func (s *Server) authorized(r *http.Request) (bool, bool) {
	if s.adminToken == "" {
		return false, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	cookieAuth := false
	if token == "" {
		if cookie, err := r.Cookie("tmdns_admin"); err == nil {
			token = cookie.Value
			cookieAuth = true
		}
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken)) == 1, cookieAuth
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || isLocalInterfaceIP(ip))
}

func isLocalInterfaceIP(ip net.IP) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		var localIP net.IP
		switch value := addr.(type) {
		case *net.IPNet:
			localIP = value.IP
		case *net.IPAddr:
			localIP = value.IP
		}
		if localIP != nil && localIP.Equal(ip) {
			return true
		}
	}
	return false
}

func sameOriginUnsafeRequest(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	ok, _ := s.authorized(r)
	if isLoopbackRequest(r) {
		ok = true
	}
	writeJSON(w, map[string]any{"authenticated": ok, "required": true})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err)
		return
	}
	if s.adminToken == "" {
		http.Error(w, "admin token unavailable", http.StatusServiceUnavailable)
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Token)), []byte(s.adminToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "tmdns_admin", Value: s.adminToken, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "tmdns_admin", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "dns": s.dns.Status(), "http_addr": s.cfg.HTTPAddr, "version": version.Info()})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Dashboard(r.Context(), s.cfg.DBPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"dashboard": d, "dns": s.dns.Status(), "system": s.systemStats(), "version": version.Info()})
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Dashboard(r.Context(), s.cfg.DBPath)
	if err != nil {
		writeError(w, err)
		return
	}
	dnsStatus := s.dns.Status()
	system := s.systemStats()
	warnings := []string{}
	if dnsStatus.DroppedEvents > 0 {
		warnings = append(warnings, "DNS query events have been dropped. Increase event queue capacity or reduce dashboard/report load.")
	}
	if dnsStatus.EventQueueDepth > s.cfg.EventQueueCap/2 {
		warnings = append(warnings, "DNS event queue is above 50% capacity.")
	}
	if system.Power.Supported && system.Power.SleepConfigured {
		warnings = append(warnings, "macOS system sleep is enabled. Disable sleep before using this Mac as a DNS appliance.")
	}
	if strings.HasPrefix(s.cfg.HTTPAddr, "0.0.0.0:") {
		warnings = append(warnings, "Admin HTTP is reachable on LAN interfaces. Use a strong admin token and prefer trusted management networks.")
	}
	writeJSON(w, map[string]any{
		"ok":        len(warnings) == 0,
		"dns":       dnsStatus,
		"dashboard": d,
		"system":    system,
		"version":   version.Info(),
		"config": map[string]any{
			"dns_addr":        s.cfg.DNSAddr,
			"http_addr":       s.cfg.HTTPAddr,
			"upstream":        s.cfg.Upstream,
			"event_queue_cap": s.cfg.EventQueueCap,
			"db_path":         s.cfg.DBPath,
		},
		"warnings": warnings,
	})
}

func (s *Server) systemStats() systemstats.Stats {
	s.statMu.Lock()
	defer s.statMu.Unlock()
	if time.Since(s.statAt) < 30*time.Second {
		return s.stat
	}
	s.stat = s.stats.Snapshot()
	s.statAt = time.Now()
	return s.stat
}

func (s *Server) realtime(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.RecentEvents(r.Context(), "", 150)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) blocked(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.RecentEvents(r.Context(), "blocked", 150)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) hosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hosts, err := s.store.Hosts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, hosts)
}

func (s *Server) hostDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/hosts/"), 10, 64)
	if err != nil {
		http.Error(w, "invalid host id", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, err)
			return
		}
		if err := s.store.SetHostLabel(r.Context(), id, body.Label); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	detail, err := s.store.HostDetail(r.Context(), id, hours)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rules, err := s.store.Rules(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, rules)
}

func (s *Server) ruleDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/rules/")
	if path == "block" || path == "allow" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid rule id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err)
		return
	}
	rule, err := s.store.SetRuleEnabled(r.Context(), id, body.Enabled)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, rule)
}

func (s *Server) ruleAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Target string `json:"target"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, err)
			return
		}
		rule, err := s.store.AddRule(r.Context(), body.Target, action, body.Note)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, rule)
	}
}

func (s *Server) blocklistPresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	presets, err := s.store.BlocklistPresets(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, presets)
}

func (s *Server) blocklistPresetDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/blocklist-presets/")
	if id == "" {
		http.Error(w, "invalid preset id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err)
		return
	}
	preset, err := s.store.SetBlocklistPresetEnabled(r.Context(), id, body.Enabled)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, preset)
}

func (s *Server) blocklistSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sources, err := s.store.BlocklistSources(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, sources)
	case http.MethodPost:
		var body struct {
			Name   string `json:"name"`
			URL    string `json:"url"`
			Format string `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, err)
			return
		}
		source, err := s.store.AddBlocklistSource(r.Context(), body.Name, body.URL, body.Format)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, source)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) blocklistSourceDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/blocklist-sources/"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid source id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err)
		return
	}
	source, err := s.store.SetBlocklistSourceEnabled(r.Context(), id, body.Enabled)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, source)
}

func (s *Server) blocklistsRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	results, err := s.store.RefreshBlocklists(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, results)
}

func (s *Server) records(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		records, err := s.store.StaticRecords(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, records)
	case http.MethodPost:
		var record store.StaticRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			writeError(w, err)
			return
		}
		if err := s.store.UpsertStaticRecord(r.Context(), record); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) hostReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/reports/host/"), 10, 64)
	if err != nil {
		http.Error(w, "invalid host id", http.StatusBadRequest)
		return
	}
	report, err := s.store.HostReport(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events, err := s.store.AuditEvents(r.Context(), 250)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) unifiSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.store.UniFiSettings(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		settings.APIKey = ""
		writeJSON(w, settings)
	case http.MethodPut:
		var body store.UniFiSettings
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, err)
			return
		}
		settings, err := s.store.SaveUniFiSettings(r.Context(), body)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, settings)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) unifiTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.store.TestUniFi(r.Context())
	if err != nil {
		writeJSON(w, result)
		return
	}
	writeJSON(w, result)
}

func (s *Server) unifiImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.store.ImportUniFiClients(r.Context())
	if err != nil {
		writeJSON(w, result)
		return
	}
	writeJSON(w, result)
}

func (s *Server) retentionSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.store.Retention(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, settings)
	case http.MethodPut:
		var body store.RetentionSettings
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, err)
			return
		}
		settings, err := s.store.SetRetention(r.Context(), body.Days)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, settings)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) retentionPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	count, err := s.store.PurgeOldEvents(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "removed": count})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func logMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("http request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
