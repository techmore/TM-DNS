package store

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

type StaticRecord struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	TTL       int       `json:"ttl"`
	CreatedAt time.Time `json:"created_at"`
}

type Rule struct {
	ID        int64      `json:"id"`
	Scope     string     `json:"scope"`
	Target    string     `json:"target"`
	Action    string     `json:"action"`
	Enabled   bool       `json:"enabled"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	HitCount  int64      `json:"hit_count"`
	LastHitAt *time.Time `json:"last_hit_at,omitempty"`
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"created_at"`
}

type BlocklistPreset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Tier        string `json:"tier"`
	Description string `json:"description"`
	HomeURL     string `json:"home_url"`
	SourceURL   string `json:"source_url"`
	Enabled     bool   `json:"enabled"`
}

type BlocklistSource struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Format      string `json:"format"`
	Enabled     bool   `json:"enabled"`
	LastStatus  string `json:"last_status"`
	LastChecked string `json:"last_checked"`
	CreatedAt   string `json:"created_at"`
}

type BlocklistRefreshResult struct {
	SourceName string `json:"source_name"`
	URL        string `json:"url"`
	Status     string `json:"status"`
	Entries    int    `json:"entries"`
	Error      string `json:"error,omitempty"`
}

type BlocklistMatch struct {
	Domain     string `json:"domain"`
	SourceName string `json:"source_name"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

type QueryEvent struct {
	Timestamp     time.Time `json:"timestamp"`
	HostID        int64     `json:"host_id"`
	SourceIP      string    `json:"source_ip"`
	HostLabel     string    `json:"host_label"`
	QueryName     string    `json:"query_name"`
	QueryType     string    `json:"query_type"`
	Action        string    `json:"action"`
	MatchedRuleID *int64    `json:"matched_rule_id,omitempty"`
	MatchedSource string    `json:"matched_source"`
	ResponseCode  string    `json:"response_code"`
	Upstream      string    `json:"upstream"`
	LatencyMS     int64     `json:"latency_ms"`
	AnswerSummary string    `json:"answer_summary"`
}

type Dashboard struct {
	QueriesToday   int64        `json:"queries_today"`
	BlockedToday   int64        `json:"blocked_today"`
	UniqueHosts    int64        `json:"unique_hosts"`
	Recent         []QueryEvent `json:"recent"`
	Blocked        []QueryEvent `json:"blocked"`
	TopHosts       []TopHostRow `json:"top_hosts"`
	TopDomains     []TopRow     `json:"top_domains"`
	RuleHits       []TopRow     `json:"rule_hits"`
	DatabasePath   string       `json:"database_path"`
	EventStoreMode string       `json:"event_store_mode"`
}

type TopRow struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type TopHostRow struct {
	ID       int64  `json:"id"`
	Key      string `json:"key"`
	SourceIP string `json:"source_ip"`
	Label    string `json:"label"`
	Hostname string `json:"hostname"`
	Count    int64  `json:"count"`
}

type Host struct {
	ID                 int64  `json:"id"`
	SourceIP           string `json:"source_ip"`
	Label              string `json:"label"`
	Hostname           string `json:"hostname"`
	MAC                string `json:"mac"`
	Group              string `json:"group"`
	IdentityConfidence string `json:"identity_confidence"`
	FirstSeen          string `json:"first_seen"`
	LastSeen           string `json:"last_seen"`
	QueryCount         int64  `json:"query_count"`
	BlockCount         int64  `json:"block_count"`
	Notes              string `json:"notes"`
}

type HostDetail struct {
	Host       Host         `json:"host"`
	Recent     []QueryEvent `json:"recent"`
	Blocked    []QueryEvent `json:"blocked"`
	TopDomains []TopRow     `json:"top_domains"`
	TopActions []TopRow     `json:"top_actions"`
}

type HostReport struct {
	Host             Host     `json:"host"`
	Window           string   `json:"window"`
	TotalQueries     int64    `json:"total_queries"`
	TotalBlocked     int64    `json:"total_blocked"`
	UniqueDomains    int64    `json:"unique_domains"`
	TopDomains       []TopRow `json:"top_domains"`
	TopBlocked       []TopRow `json:"top_blocked"`
	Actions          []TopRow `json:"actions"`
	FirstEventAt     string   `json:"first_event_at"`
	LastEventAt      string   `json:"last_event_at"`
	RecommendedNotes []string `json:"recommended_notes"`
}

type AuditEvent struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Detail    string `json:"detail"`
}

func Open(ctx context.Context, path string, logger *slog.Logger) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db, logger: logger}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
		`PRAGMA busy_timeout=5000;`,
		`PRAGMA foreign_keys=ON;`,
		`CREATE TABLE IF NOT EXISTS hosts (
			id INTEGER PRIMARY KEY,
			source_ip TEXT NOT NULL UNIQUE,
			label TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT '',
			mac TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT 'Default',
			identity_confidence TEXT NOT NULL DEFAULT 'source_ip',
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			query_count INTEGER NOT NULL DEFAULT 0,
			block_count INTEGER NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS host_observations (
			id INTEGER PRIMARY KEY,
			host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
			source TEXT NOT NULL,
			value TEXT NOT NULL,
			observed_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS static_records (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			value TEXT NOT NULL,
			ttl INTEGER NOT NULL DEFAULT 60,
			created_at TEXT NOT NULL,
			UNIQUE(name, type)
		);`,
		`CREATE TABLE IF NOT EXISTS rules (
			id INTEGER PRIMARY KEY,
			scope TEXT NOT NULL DEFAULT 'global',
			target TEXT NOT NULL,
			action TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			expires_at TEXT,
			hit_count INTEGER NOT NULL DEFAULT 0,
			last_hit_at TEXT,
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`DELETE FROM rules
			WHERE id NOT IN (
				SELECT MIN(id) FROM rules GROUP BY scope, target, action
			);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_rules_target_action_scope ON rules(scope, target, action);`,
		`CREATE TABLE IF NOT EXISTS blocklist_presets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			tier TEXT NOT NULL,
			description TEXT NOT NULL,
			home_url TEXT NOT NULL,
			source_url TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS blocklist_sources (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			format TEXT NOT NULL DEFAULT 'domains',
			enabled INTEGER NOT NULL DEFAULT 0,
			last_status TEXT NOT NULL DEFAULT 'not checked',
			last_checked TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS blocklist_entries (
			domain TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_name TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(domain, source_type, source_id)
		);`,
		`CREATE TABLE IF NOT EXISTS query_events (
			id INTEGER PRIMARY KEY,
			ts TEXT NOT NULL,
			host_id INTEGER NOT NULL REFERENCES hosts(id),
			source_ip TEXT NOT NULL,
			query_name TEXT NOT NULL,
			query_type TEXT NOT NULL,
			action TEXT NOT NULL,
			matched_rule_id INTEGER,
			matched_source TEXT NOT NULL DEFAULT '',
			response_code TEXT NOT NULL,
			upstream TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL,
			answer_summary TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS block_events (
			id INTEGER PRIMARY KEY,
			query_event_id INTEGER NOT NULL REFERENCES query_events(id) ON DELETE CASCADE,
			host_id INTEGER NOT NULL REFERENCES hosts(id),
			source_ip TEXT NOT NULL,
			query_name TEXT NOT NULL,
			matched_rule_id INTEGER,
			matched_source TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id INTEGER PRIMARY KEY,
			ts TEXT NOT NULL,
			action TEXT NOT NULL,
			target TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS system_samples (
			id INTEGER PRIMARY KEY,
			ts TEXT NOT NULL,
			key TEXT NOT NULL,
			value REAL NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_query_events_ts ON query_events(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_query_events_host ON query_events(host_id, ts);`,
		`CREATE INDEX IF NOT EXISTS idx_query_events_name ON query_events(query_name);`,
		`CREATE INDEX IF NOT EXISTS idx_block_events_host ON block_events(host_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_blocklist_entries_domain ON blocklist_entries(domain);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate %q: %w", statement, err)
		}
	}
	return nil
}

func (s *Store) SeedDefaults(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO static_records(name, type, value, ttl, created_at) VALUES
		('router.test.', 'A', '192.168.1.1', 60, ?),
		('dashboard.test.', 'A', '127.0.0.1', 60, ?)`, now, now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO rules(scope, target, action, enabled, note, created_at)
		SELECT 'global', 'blocked.test.', 'block', 1, 'Default test block rule', ?
		WHERE NOT EXISTS (SELECT 1 FROM rules WHERE target = 'blocked.test.' AND action = 'block')`, now)
	if err != nil {
		return err
	}
	return s.SeedBlocklistPresets(ctx)
}

func (s *Store) StaticRecords(ctx context.Context) ([]StaticRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, value, ttl, created_at FROM static_records ORDER BY name, type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []StaticRecord
	for rows.Next() {
		var r StaticRecord
		var created string
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Value, &r.TTL, &created); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) UpsertStaticRecord(ctx context.Context, r StaticRecord) error {
	name := NormalizeName(r.Name)
	recordType := strings.ToUpper(strings.TrimSpace(r.Type))
	if name == "" || recordType == "" || strings.TrimSpace(r.Value) == "" {
		return errors.New("name, type, and value are required")
	}
	if r.TTL <= 0 {
		r.TTL = 60
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO static_records(name, type, value, ttl, created_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(name, type) DO UPDATE SET value = excluded.value, ttl = excluded.ttl`,
		name, recordType, strings.TrimSpace(r.Value), r.TTL, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, scope, target, action, enabled, expires_at, hit_count, last_hit_at, note, created_at
		FROM rules ORDER BY enabled DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) AddRule(ctx context.Context, target, action, note string) (Rule, error) {
	target = NormalizeName(target)
	action = strings.ToLower(strings.TrimSpace(action))
	if target == "" {
		return Rule{}, errors.New("target is required")
	}
	if action != "block" && action != "allow" {
		return Rule{}, errors.New("action must be block or allow")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO rules(scope, target, action, enabled, note, created_at)
		VALUES('global', ?, ?, 1, ?, ?)
		ON CONFLICT(scope, target, action) DO UPDATE SET enabled = 1, note = CASE WHEN excluded.note != '' THEN excluded.note ELSE rules.note END`,
		target, action, note, now)
	if err != nil {
		return Rule{}, err
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM rules WHERE scope = 'global' AND target = ? AND action = ?`, target, action).Scan(&id); err != nil {
		return Rule{}, err
	}
	_ = s.Audit(ctx, "rule.create", target, action)
	return s.Rule(ctx, id)
}

func (s *Store) Rule(ctx context.Context, id int64) (Rule, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, scope, target, action, enabled, expires_at, hit_count, last_hit_at, note, created_at FROM rules WHERE id = ?`, id)
	return scanRule(row)
}

func (s *Store) SetRuleEnabled(ctx context.Context, id int64, enabled bool) (Rule, error) {
	value := 0
	if enabled {
		value = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE rules SET enabled = ? WHERE id = ?`, value, id)
	if err != nil {
		return Rule{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return Rule{}, sql.ErrNoRows
	}
	_ = s.Audit(ctx, "rule.enabled", fmt.Sprintf("%d", id), fmt.Sprintf("%t", enabled))
	return s.Rule(ctx, id)
}

func (s *Store) MatchRule(ctx context.Context, qname string) (*Rule, error) {
	qname = NormalizeName(qname)
	rules, err := s.Rules(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.ExpiresAt != nil && now.After(*rule.ExpiresAt) {
			continue
		}
		if qname == rule.Target || strings.HasSuffix(qname, "."+rule.Target) {
			return &rule, nil
		}
	}
	return nil, nil
}

func (s *Store) MatchBlocklist(ctx context.Context, qname string) (*BlocklistMatch, error) {
	qname = NormalizeName(qname)
	for candidate := qname; candidate != ""; candidate = parentDomain(candidate) {
		var match BlocklistMatch
		err := s.db.QueryRowContext(ctx, `SELECT domain, source_name, source_type, source_id FROM blocklist_entries WHERE domain = ? LIMIT 1`, candidate).
			Scan(&match.Domain, &match.SourceName, &match.SourceType, &match.SourceID)
		if err == nil {
			return &match, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

func parentDomain(name string) string {
	name = strings.TrimSuffix(name, ".")
	idx := strings.IndexByte(name, '.')
	if idx < 0 {
		return ""
	}
	return name[idx+1:] + "."
}

func (s *Store) SeedBlocklistPresets(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	presets := []BlocklistPreset{
		{ID: "hagezi-pro", Name: "HaGeZi Pro", Tier: "Balanced", Description: "Comprehensive ads, trackers, malware, phishing, and scam protection. Strong default candidate after testing.", HomeURL: "https://github.com/hagezi/dns-blocklists", SourceURL: "https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/domains/pro.txt"},
		{ID: "hagezi-light", Name: "HaGeZi Light", Tier: "Conservative", Description: "Lighter HaGeZi tier for lower false-positive risk while evaluating policy impact.", HomeURL: "https://github.com/hagezi/dns-blocklists", SourceURL: "https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/domains/light.txt"},
		{ID: "stevenblack", Name: "StevenBlack Unified Hosts", Tier: "Classic", Description: "Long-running aggregated hosts list. Good baseline for ads and malware with optional category variants.", HomeURL: "https://github.com/StevenBlack/hosts", SourceURL: "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"},
		{ID: "firebog", Name: "Firebog", Tier: "Collection", Description: "Curated collection reference for choosing individual safer or aggressive lists. Add selected raw URLs as custom sources.", HomeURL: "https://firebog.net/", SourceURL: "https://firebog.net/"},
		{ID: "blocklistproject", Name: "Block List Project Malware", Tier: "Categories", Description: "Category-specific malware list from Block List Project. Add other category URLs as custom sources when needed.", HomeURL: "https://blocklistproject.github.io/Lists/", SourceURL: "https://blocklistproject.github.io/Lists/malware.txt"},
		{ID: "oisd", Name: "OISD", Tier: "Low breakage", Description: "Popular optimized list focused on low false positives for ads, trackers, and malware.", HomeURL: "https://oisd.nl/", SourceURL: "https://big.oisd.nl/domainswild"},
		{ID: "adguard-dns", Name: "AdGuard DNS Filters", Tier: "Vendor", Description: "Official AdGuard-maintained filters. Useful comparison source for ads and tracking coverage.", HomeURL: "https://github.com/AdguardTeam/AdGuardSDNSFilter", SourceURL: "https://raw.githubusercontent.com/AdguardTeam/AdGuardSDNSFilter/master/Filters/filter.txt"},
		{ID: "urlhaus", Name: "URLhaus", Tier: "Security", Description: "Malware URL intelligence source for higher-risk security blocking evaluation.", HomeURL: "https://urlhaus.abuse.ch/", SourceURL: "https://urlhaus.abuse.ch/downloads/hostfile/"},
		{ID: "phishing-army", Name: "Phishing Army", Tier: "Security", Description: "Phishing-focused feed for security policy evaluation.", HomeURL: "https://phishing.army/", SourceURL: "https://phishing.army/download/phishing_army_blocklist_extended.txt"},
	}
	for _, preset := range presets {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO blocklist_presets(id, name, tier, description, home_url, source_url, enabled, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, 0, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, tier = excluded.tier, description = excluded.description, home_url = excluded.home_url, source_url = excluded.source_url, updated_at = excluded.updated_at`,
			preset.ID, preset.Name, preset.Tier, preset.Description, preset.HomeURL, preset.SourceURL, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BlocklistPresets(ctx context.Context) ([]BlocklistPreset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, tier, description, home_url, source_url, enabled FROM blocklist_presets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var presets []BlocklistPreset
	for rows.Next() {
		var preset BlocklistPreset
		var enabled int
		if err := rows.Scan(&preset.ID, &preset.Name, &preset.Tier, &preset.Description, &preset.HomeURL, &preset.SourceURL, &enabled); err != nil {
			return nil, err
		}
		preset.Enabled = enabled == 1
		presets = append(presets, preset)
	}
	return presets, rows.Err()
}

func (s *Store) SetBlocklistPresetEnabled(ctx context.Context, id string, enabled bool) (BlocklistPreset, error) {
	value := 0
	if enabled {
		value = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE blocklist_presets SET enabled = ?, updated_at = ? WHERE id = ?`, value, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return BlocklistPreset{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return BlocklistPreset{}, sql.ErrNoRows
	}
	_ = s.Audit(ctx, "blocklist.enabled", id, fmt.Sprintf("%t", enabled))
	var preset BlocklistPreset
	var enabledInt int
	err = s.db.QueryRowContext(ctx, `SELECT id, name, tier, description, home_url, source_url, enabled FROM blocklist_presets WHERE id = ?`, id).
		Scan(&preset.ID, &preset.Name, &preset.Tier, &preset.Description, &preset.HomeURL, &preset.SourceURL, &enabledInt)
	preset.Enabled = enabledInt == 1
	return preset, err
}

func (s *Store) BlocklistSources(ctx context.Context) ([]BlocklistSource, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, url, format, enabled, last_status, last_checked, created_at FROM blocklist_sources ORDER BY enabled DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []BlocklistSource
	for rows.Next() {
		var source BlocklistSource
		var enabled int
		if err := rows.Scan(&source.ID, &source.Name, &source.URL, &source.Format, &enabled, &source.LastStatus, &source.LastChecked, &source.CreatedAt); err != nil {
			return nil, err
		}
		source.Enabled = enabled == 1
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Store) AddBlocklistSource(ctx context.Context, name, url, format string) (BlocklistSource, error) {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	format = strings.ToLower(strings.TrimSpace(format))
	if name == "" {
		name = url
	}
	if url == "" {
		return BlocklistSource{}, errors.New("url is required")
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return BlocklistSource{}, errors.New("url must start with http:// or https://")
	}
	if format == "" {
		format = "domains"
	}
	if format != "domains" && format != "hosts" && format != "adguard" {
		return BlocklistSource{}, errors.New("format must be domains, hosts, or adguard")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO blocklist_sources(name, url, format, enabled, created_at)
		VALUES(?, ?, ?, 1, ?)
		ON CONFLICT(url) DO UPDATE SET name = excluded.name, format = excluded.format, enabled = 1`,
		name, url, format, now)
	if err != nil {
		return BlocklistSource{}, err
	}
	_ = s.Audit(ctx, "blocklist.source.add", url, name)
	return s.blocklistSourceByURL(ctx, url)
}

func (s *Store) SetBlocklistSourceEnabled(ctx context.Context, id int64, enabled bool) (BlocklistSource, error) {
	value := 0
	if enabled {
		value = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE blocklist_sources SET enabled = ? WHERE id = ?`, value, id)
	if err != nil {
		return BlocklistSource{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return BlocklistSource{}, sql.ErrNoRows
	}
	_ = s.Audit(ctx, "blocklist.source.enabled", fmt.Sprintf("%d", id), fmt.Sprintf("%t", enabled))
	return s.BlocklistSource(ctx, id)
}

func (s *Store) BlocklistSource(ctx context.Context, id int64) (BlocklistSource, error) {
	var source BlocklistSource
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT id, name, url, format, enabled, last_status, last_checked, created_at FROM blocklist_sources WHERE id = ?`, id).
		Scan(&source.ID, &source.Name, &source.URL, &source.Format, &enabled, &source.LastStatus, &source.LastChecked, &source.CreatedAt)
	source.Enabled = enabled == 1
	return source, err
}

func (s *Store) blocklistSourceByURL(ctx context.Context, url string) (BlocklistSource, error) {
	var source BlocklistSource
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT id, name, url, format, enabled, last_status, last_checked, created_at FROM blocklist_sources WHERE url = ?`, url).
		Scan(&source.ID, &source.Name, &source.URL, &source.Format, &enabled, &source.LastStatus, &source.LastChecked, &source.CreatedAt)
	source.Enabled = enabled == 1
	return source, err
}

func (s *Store) RefreshBlocklists(ctx context.Context) ([]BlocklistRefreshResult, error) {
	sources, err := s.enabledBlocklistFetchSources(ctx)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 45 * time.Second}
	results := make([]BlocklistRefreshResult, 0, len(sources))
	for _, source := range sources {
		result := s.refreshBlocklistSource(ctx, client, source)
		results = append(results, result)
	}
	_ = s.Audit(ctx, "blocklist.refresh", "all", fmt.Sprintf("%d sources", len(results)))
	return results, nil
}

type blocklistFetchSource struct {
	ID         string
	Type       string
	Name       string
	URL        string
	Format     string
	CustomID   int64
	CustomType bool
}

func (s *Store) enabledBlocklistFetchSources(ctx context.Context) ([]blocklistFetchSource, error) {
	var out []blocklistFetchSource
	presets, err := s.BlocklistPresets(ctx)
	if err != nil {
		return nil, err
	}
	for _, preset := range presets {
		if !preset.Enabled {
			continue
		}
		out = append(out, blocklistFetchSource{ID: preset.ID, Type: "preset", Name: preset.Name, URL: preset.SourceURL, Format: inferBlocklistFormat(preset.SourceURL)})
	}
	custom, err := s.BlocklistSources(ctx)
	if err != nil {
		return nil, err
	}
	for _, source := range custom {
		if !source.Enabled {
			continue
		}
		out = append(out, blocklistFetchSource{ID: fmt.Sprintf("%d", source.ID), Type: "custom", Name: source.Name, URL: source.URL, Format: source.Format, CustomID: source.ID, CustomType: true})
	}
	return out, nil
}

func (s *Store) refreshBlocklistSource(ctx context.Context, client *http.Client, source blocklistFetchSource) BlocklistRefreshResult {
	result := BlocklistRefreshResult{SourceName: source.Name, URL: source.URL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		result.Status = "invalid url"
		result.Error = err.Error()
		s.recordBlocklistSourceStatus(ctx, source, result)
		return result
	}
	req.Header.Set("User-Agent", "TM-DNS/0.1")
	resp, err := client.Do(req)
	if err != nil {
		result.Status = "fetch failed"
		result.Error = err.Error()
		s.recordBlocklistSourceStatus(ctx, source, result)
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		result.Status = fmt.Sprintf("http %d", resp.StatusCode)
		s.recordBlocklistSourceStatus(ctx, source, result)
		return result
	}
	limited := io.LimitReader(resp.Body, 50*1024*1024)
	domains, err := parseBlocklistDomains(limited, source.Format)
	if err != nil {
		result.Status = "parse failed"
		result.Error = err.Error()
		s.recordBlocklistSourceStatus(ctx, source, result)
		return result
	}
	if err := s.replaceBlocklistEntries(ctx, source, domains); err != nil {
		result.Status = "store failed"
		result.Error = err.Error()
		s.recordBlocklistSourceStatus(ctx, source, result)
		return result
	}
	result.Status = "ok"
	result.Entries = len(domains)
	s.recordBlocklistSourceStatus(ctx, source, result)
	return result
}

func (s *Store) replaceBlocklistEntries(ctx context.Context, source blocklistFetchSource, domains map[string]struct{}) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM blocklist_entries WHERE source_type = ? AND source_id = ?`, source.Type, source.ID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO blocklist_entries(domain, source_type, source_id, source_name, updated_at) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for domain := range domains {
		if _, err := stmt.ExecContext(ctx, domain, source.Type, source.ID, source.Name, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) recordBlocklistSourceStatus(ctx context.Context, source blocklistFetchSource, result BlocklistRefreshResult) {
	if source.CustomType {
		_, _ = s.db.ExecContext(ctx, `UPDATE blocklist_sources SET last_status = ?, last_checked = ? WHERE id = ?`, result.Status, time.Now().UTC().Format(time.RFC3339Nano), source.CustomID)
	}
}

var domainTokenRE = regexp.MustCompile(`(?i)^([a-z0-9_*.-]+\.)+[a-z]{2,}\.?$`)

func parseBlocklistDomains(r io.Reader, format string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(line, "||") {
			line = strings.TrimPrefix(line, "||")
			if idx := strings.IndexAny(line, "^/$"); idx >= 0 {
				line = line[:idx]
			}
		}
		if strings.Contains(line, " ") || strings.Contains(line, "\t") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && (net.ParseIP(fields[0]) != nil || fields[0] == "0.0.0.0" || fields[0] == "::") {
				line = fields[1]
			} else {
				line = fields[0]
			}
		}
		line = strings.TrimPrefix(line, "address=/")
		if idx := strings.Index(line, "/"); idx > 0 && strings.Contains(line[idx+1:], ".") {
			line = line[:idx]
		}
		line = strings.TrimPrefix(line, "*.")
		line = strings.TrimPrefix(line, ".")
		line = strings.TrimSpace(line)
		if strings.ContainsAny(line, "/:") || strings.Contains(line, "$") || strings.Contains(line, "@@") {
			continue
		}
		if !domainTokenRE.MatchString(line) {
			continue
		}
		if normalized := NormalizeName(line); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out, scanner.Err()
}

func inferBlocklistFormat(url string) string {
	if strings.Contains(url, "AdGuard") || strings.Contains(url, "filter.txt") {
		return "adguard"
	}
	if strings.Contains(url, "hosts") || strings.Contains(url, "hostfile") {
		return "hosts"
	}
	return "domains"
}

func (s *Store) RecordRuleHit(ctx context.Context, id int64) {
	_, err := s.db.ExecContext(ctx, `UPDATE rules SET hit_count = hit_count + 1, last_hit_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		s.logger.Warn("record rule hit failed", "rule_id", id, "error", err)
	}
}

func (s *Store) EnsureHost(ctx context.Context, sourceIP string) (int64, string, error) {
	sourceIP = strings.TrimSpace(sourceIP)
	if net.ParseIP(sourceIP) == nil {
		sourceIP = "unknown"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO hosts(source_ip, first_seen, last_seen) VALUES(?, ?, ?)`, sourceIP, now, now)
	if err != nil {
		return 0, "", err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE hosts SET last_seen = ? WHERE source_ip = ?`, now, sourceIP)
	if err != nil {
		return 0, "", err
	}
	var id int64
	var label, hostname string
	err = s.db.QueryRowContext(ctx, `SELECT id, label, hostname FROM hosts WHERE source_ip = ?`, sourceIP).Scan(&id, &label, &hostname)
	if err != nil {
		return 0, "", err
	}
	display := sourceIP
	if label != "" {
		display = label
	} else if hostname != "" {
		display = hostname
	}
	return id, display, nil
}

func (s *Store) SetHostLabel(ctx context.Context, id int64, label string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosts SET label = ? WHERE id = ?`, strings.TrimSpace(label), id)
	if err == nil {
		_ = s.Audit(ctx, "host.label", fmt.Sprintf("%d", id), label)
	}
	return err
}

func (s *Store) InsertQueryEvent(ctx context.Context, event QueryEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO query_events(ts, host_id, source_ip, query_name, query_type, action, matched_rule_id, matched_source, response_code, upstream, latency_ms, answer_summary)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.Timestamp.UTC().Format(time.RFC3339Nano), event.HostID, event.SourceIP, event.QueryName, event.QueryType, event.Action, event.MatchedRuleID, event.MatchedSource, event.ResponseCode, event.Upstream, event.LatencyMS, event.AnswerSummary)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE hosts SET query_count = query_count + 1, block_count = block_count + CASE WHEN ? = 'blocked' THEN 1 ELSE 0 END WHERE id = ?`, event.Action, event.HostID)
	if err != nil {
		return err
	}
	if event.Action == "blocked" {
		id, _ := res.LastInsertId()
		_, err = tx.ExecContext(ctx, `INSERT INTO block_events(query_event_id, host_id, source_ip, query_name, matched_rule_id, matched_source, created_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)`, id, event.HostID, event.SourceIP, event.QueryName, event.MatchedRuleID, event.MatchedSource, event.Timestamp.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RecentEvents(ctx context.Context, action string, limit int) ([]QueryEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where := ""
	args := []any{}
	if action != "" {
		where = "WHERE qe.action = ?"
		args = append(args, action)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT qe.ts, qe.host_id, qe.source_ip, COALESCE(NULLIF(h.label, ''), NULLIF(h.hostname, ''), qe.source_ip), qe.query_name, qe.query_type, qe.action, qe.matched_rule_id, qe.matched_source, qe.response_code, qe.upstream, qe.latency_ms, qe.answer_summary
		FROM query_events qe JOIN hosts h ON h.id = qe.host_id `+where+` ORDER BY qe.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) Dashboard(ctx context.Context, dbPath string) (Dashboard, error) {
	var d Dashboard
	d.DatabasePath = dbPath
	d.EventStoreMode = "sqlite-wal"
	today := time.Now().Format("2006-01-02") + "T00:00:00"
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM query_events WHERE ts >= ?`, today).Scan(&d.QueriesToday)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM query_events WHERE ts >= ? AND action = 'blocked'`, today).Scan(&d.BlockedToday)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT host_id) FROM query_events WHERE ts >= ?`, today).Scan(&d.UniqueHosts)
	d.Recent, _ = s.RecentEvents(ctx, "", 30)
	d.Blocked, _ = s.RecentEvents(ctx, "blocked", 30)
	d.TopHosts, _ = s.topHostRows(ctx, today)
	d.TopDomains, _ = s.topRows(ctx, `SELECT query_name, COUNT(*) FROM query_events WHERE ts >= ? GROUP BY query_name ORDER BY COUNT(*) DESC LIMIT 8`, today)
	d.RuleHits, _ = s.topRows(ctx, `SELECT target || ' (' || action || ')', hit_count FROM rules WHERE hit_count > 0 ORDER BY hit_count DESC LIMIT 8`)
	return d, nil
}

func (s *Store) Hosts(ctx context.Context) ([]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, source_ip, label, hostname, mac, group_name, identity_confidence, first_seen, last_seen, query_count, block_count, notes FROM hosts ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []Host
	for rows.Next() {
		var host Host
		if err := rows.Scan(&host.ID, &host.SourceIP, &host.Label, &host.Hostname, &host.MAC, &host.Group, &host.IdentityConfidence, &host.FirstSeen, &host.LastSeen, &host.QueryCount, &host.BlockCount, &host.Notes); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, rows.Err()
}

func (s *Store) Host(ctx context.Context, id int64) (Host, error) {
	var host Host
	err := s.db.QueryRowContext(ctx, `SELECT id, source_ip, label, hostname, mac, group_name, identity_confidence, first_seen, last_seen, query_count, block_count, notes FROM hosts WHERE id = ?`, id).
		Scan(&host.ID, &host.SourceIP, &host.Label, &host.Hostname, &host.MAC, &host.Group, &host.IdentityConfidence, &host.FirstSeen, &host.LastSeen, &host.QueryCount, &host.BlockCount, &host.Notes)
	return host, err
}

func (s *Store) HostDetail(ctx context.Context, id int64) (HostDetail, error) {
	host, err := s.Host(ctx, id)
	if err != nil {
		return HostDetail{}, err
	}
	recent, err := s.RecentEventsByHost(ctx, id, "", 100)
	if err != nil {
		return HostDetail{}, err
	}
	blocked, err := s.RecentEventsByHost(ctx, id, "blocked", 100)
	if err != nil {
		return HostDetail{}, err
	}
	topDomains, _ := s.topRows(ctx, `SELECT query_name, COUNT(*) FROM query_events WHERE host_id = ? GROUP BY query_name ORDER BY COUNT(*) DESC LIMIT 12`, id)
	topActions, _ := s.topRows(ctx, `SELECT action, COUNT(*) FROM query_events WHERE host_id = ? GROUP BY action ORDER BY COUNT(*) DESC`, id)
	return HostDetail{Host: host, Recent: recent, Blocked: blocked, TopDomains: topDomains, TopActions: topActions}, nil
}

func (s *Store) HostReport(ctx context.Context, id int64) (HostReport, error) {
	host, err := s.Host(ctx, id)
	if err != nil {
		return HostReport{}, err
	}
	report := HostReport{Host: host, Window: "all"}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT query_name), COALESCE(MIN(ts), ''), COALESCE(MAX(ts), '') FROM query_events WHERE host_id = ?`, id).
		Scan(&report.TotalQueries, &report.UniqueDomains, &report.FirstEventAt, &report.LastEventAt)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM query_events WHERE host_id = ? AND action = 'blocked'`, id).Scan(&report.TotalBlocked)
	report.TopDomains, _ = s.topRows(ctx, `SELECT query_name, COUNT(*) FROM query_events WHERE host_id = ? GROUP BY query_name ORDER BY COUNT(*) DESC LIMIT 10`, id)
	report.TopBlocked, _ = s.topRows(ctx, `SELECT query_name, COUNT(*) FROM query_events WHERE host_id = ? AND action = 'blocked' GROUP BY query_name ORDER BY COUNT(*) DESC LIMIT 10`, id)
	report.Actions, _ = s.topRows(ctx, `SELECT action, COUNT(*) FROM query_events WHERE host_id = ? GROUP BY action ORDER BY COUNT(*) DESC`, id)
	report.RecommendedNotes = hostReportNotes(report)
	return report, nil
}

func (s *Store) RecentEventsByHost(ctx context.Context, hostID int64, action string, limit int) ([]QueryEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where := "WHERE qe.host_id = ?"
	args := []any{hostID}
	if action != "" {
		where += " AND qe.action = ?"
		args = append(args, action)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT qe.ts, qe.host_id, qe.source_ip, COALESCE(NULLIF(h.label, ''), NULLIF(h.hostname, ''), qe.source_ip), qe.query_name, qe.query_type, qe.action, qe.matched_rule_id, qe.matched_source, qe.response_code, qe.upstream, qe.latency_ms, qe.answer_summary
		FROM query_events qe JOIN hosts h ON h.id = qe.host_id `+where+` ORDER BY qe.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) Audit(ctx context.Context, action, target, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(ts, action, target, detail) VALUES(?, ?, ?, ?)`, time.Now().UTC().Format(time.RFC3339Nano), action, target, detail)
	return err
}

func (s *Store) AuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, action, target, detail FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.Action, &event.Target, &event.Detail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func hostReportNotes(report HostReport) []string {
	notes := []string{}
	if report.TotalQueries == 0 {
		return append(notes, "No DNS activity has been recorded for this host yet.")
	}
	if report.TotalBlocked > 0 {
		notes = append(notes, "Review blocked domains and rule attribution before adding broad allow rules.")
	}
	if report.UniqueDomains > 50 {
		notes = append(notes, "High unique-domain count for the selected window; inspect newly seen domains and top categories.")
	}
	if len(notes) == 0 {
		notes = append(notes, "No immediate DNS policy concerns were detected from the recorded activity.")
	}
	return notes
}

func (s *Store) topRows(ctx context.Context, query string, args ...any) ([]TopRow, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopRow
	for rows.Next() {
		var row TopRow
		if err := rows.Scan(&row.Key, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) topHostRows(ctx context.Context, since string) ([]TopHostRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT h.id, COALESCE(NULLIF(h.label, ''), NULLIF(h.hostname, ''), h.source_ip), h.source_ip, h.label, h.hostname, COUNT(*)
		FROM query_events qe JOIN hosts h ON h.id = qe.host_id
		WHERE qe.ts >= ?
		GROUP BY qe.host_id
		ORDER BY COUNT(*) DESC
		LIMIT 8`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopHostRow
	for rows.Next() {
		var row TopHostRow
		if err := rows.Scan(&row.ID, &row.Key, &row.SourceIP, &row.Label, &row.Hostname, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type ruleScanner interface {
	Scan(dest ...any) error
}

func scanRule(scanner ruleScanner) (Rule, error) {
	var r Rule
	var enabled int
	var expires, lastHit, created sql.NullString
	if err := scanner.Scan(&r.ID, &r.Scope, &r.Target, &r.Action, &enabled, &expires, &r.HitCount, &lastHit, &r.Note, &created); err != nil {
		return Rule{}, err
	}
	r.Enabled = enabled == 1
	if expires.Valid && expires.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, expires.String)
		r.ExpiresAt = &t
	}
	if lastHit.Valid && lastHit.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, lastHit.String)
		r.LastHitAt = &t
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	return r, nil
}

func scanEvents(rows *sql.Rows) ([]QueryEvent, error) {
	var events []QueryEvent
	for rows.Next() {
		var event QueryEvent
		var ts string
		var ruleID sql.NullInt64
		if err := rows.Scan(&ts, &event.HostID, &event.SourceIP, &event.HostLabel, &event.QueryName, &event.QueryType, &event.Action, &ruleID, &event.MatchedSource, &event.ResponseCode, &event.Upstream, &event.LatencyMS, &event.AnswerSummary); err != nil {
			return nil, err
		}
		event.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if ruleID.Valid {
			event.MatchedRuleID = &ruleID.Int64
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func NormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, ".")
	if name == "" {
		return ""
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}
