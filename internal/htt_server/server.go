package htt_server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/techmore/tm-dns/internal/config"
	"github.com/techmore/tm-dns/internal/dnsserver"
	"github.com/techmore/tm-dns/internal/store"
	"github.com/techmore/tm-dns/internal/systemstats"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	dns    *dnsserver.Server
	logger *slog.Logger
	server *http.Server
	mux    *http.ServeMux
	stats  *systemstats.Sampler
	statMu sync.Mutex
	statAt time.Time
	stat   systemstats.Stats
}

func New(cfg config.Config, st *store.Store, dns *dnsserver.Server, logger *slog.Logger) *Server {
	s := &Server{cfg: cfg, store: st, dns: dns, logger: logger, mux: http.NewServeMux(), stats: systemstats.NewSampler(cfg.DBPath)}
	s.routes()
	s.server = &http.Server{Addr: cfg.HTTPAddr, Handler: logMiddleware(logger, s.mux)}
	return s
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
	s.mux.HandleFunc("/api/health", s.health)
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
	writeJSON(w, map[string]any{"ok": true, "dns": s.dns.Status(), "http_addr": s.cfg.HTTPAddr})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Dashboard(r.Context(), s.cfg.DBPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"dashboard": d, "dns": s.dns.Status(), "system": s.systemStats()})
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
	detail, err := s.store.HostDetail(r.Context(), id)
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
