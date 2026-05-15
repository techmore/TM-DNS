package store

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db              *sql.DB
	logger          *slog.Logger
	secretKey       []byte
	cacheMu         sync.RWMutex
	rulesCache      []Rule
	rulesLoaded     bool
	staticCache     []StaticRecord
	staticLoaded    bool
	blocklistCache  map[string]BlocklistMatch
	blocklistLoaded bool
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
	RetentionDays  int          `json:"retention_days"`
}

type TopRow struct {
	Key     string  `json:"key"`
	Count   int64   `json:"count"`
	Percent float64 `json:"percent,omitempty"`
}

type TopHostRow struct {
	ID       int64  `json:"id"`
	Key      string `json:"key"`
	SourceIP string `json:"source_ip"`
	Label    string `json:"label"`
	Hostname string `json:"hostname"`
	MAC      string `json:"mac"`
	Vendor   string `json:"vendor"`
	Count    int64  `json:"count"`
}

type Host struct {
	ID                  int64  `json:"id"`
	SourceIP            string `json:"source_ip"`
	Label               string `json:"label"`
	Hostname            string `json:"hostname"`
	MAC                 string `json:"mac"`
	Vendor              string `json:"vendor"`
	Group               string `json:"group"`
	IdentityConfidence  string `json:"identity_confidence"`
	IdentityLastChecked string `json:"identity_last_checked"`
	FirstSeen           string `json:"first_seen"`
	LastSeen            string `json:"last_seen"`
	QueryCount          int64  `json:"query_count"`
	BlockCount          int64  `json:"block_count"`
	Notes               string `json:"notes"`
}

type HostDetail struct {
	Host          Host         `json:"host"`
	WindowHours   int          `json:"window_hours"`
	TotalQueries  int64        `json:"total_queries"`
	TotalBlocked  int64        `json:"total_blocked"`
	UniqueDomains int64        `json:"unique_domains"`
	Recent        []QueryEvent `json:"recent"`
	Blocked       []QueryEvent `json:"blocked"`
	TopDomains    []TopRow     `json:"top_domains"`
	TopActions    []TopRow     `json:"top_actions"`
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

type UniFiSettings struct {
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	Site       string `json:"site"`
	APIKey     string `json:"api_key,omitempty"`
	HasAPIKey  bool   `json:"has_api_key"`
	LastImport string `json:"last_import"`
	LastStatus string `json:"last_status"`
}

type UniFiImportResult struct {
	Status  string `json:"status"`
	Seen    int    `json:"seen"`
	Updated int    `json:"updated"`
	Error   string `json:"error,omitempty"`
}

type RetentionSettings struct {
	Days      int    `json:"days"`
	LastPurge string `json:"last_purge"`
}

type HASettings struct {
	Enabled       bool   `json:"enabled"`
	Role          string `json:"role"`
	PeerName      string `json:"peer_name"`
	PeerURL       string `json:"peer_url"`
	PeerToken     string `json:"peer_token,omitempty"`
	HasPeerToken  bool   `json:"has_peer_token"`
	LastHeartbeat string `json:"last_heartbeat"`
	LastSync      string `json:"last_sync"`
	LastStatus    string `json:"last_status"`
}

type HAStatus struct {
	Status      string `json:"status"`
	PeerURL     string `json:"peer_url"`
	PeerVersion string `json:"peer_version,omitempty"`
	PeerRole    string `json:"peer_role,omitempty"`
	Error       string `json:"error,omitempty"`
	CheckedAt   string `json:"checked_at"`
}

type HAHealth struct {
	Enabled             bool   `json:"enabled"`
	Role                string `json:"role"`
	PeerName            string `json:"peer_name"`
	PeerURL             string `json:"peer_url"`
	Status              string `json:"status"`
	LastHeartbeat       string `json:"last_heartbeat"`
	LastSync            string `json:"last_sync"`
	HeartbeatAgeSeconds int64  `json:"heartbeat_age_seconds"`
	Stale               bool   `json:"stale"`
	Configured          bool   `json:"configured"`
}

type HASyncPayload struct {
	StaticRecords     []StaticRecord    `json:"static_records"`
	Rules             []Rule            `json:"rules"`
	BlocklistPresets  []BlocklistPreset `json:"blocklist_presets"`
	BlocklistSources  []BlocklistSource `json:"blocklist_sources"`
	RetentionSettings RetentionSettings `json:"retention_settings"`
	SyncedAt          string            `json:"synced_at"`
}

type HASyncResult struct {
	Status           string `json:"status"`
	PeerURL          string `json:"peer_url"`
	StaticRecords    int    `json:"static_records"`
	Rules            int    `json:"rules"`
	BlocklistPresets int    `json:"blocklist_presets"`
	BlocklistSources int    `json:"blocklist_sources"`
	Error            string `json:"error,omitempty"`
}

type HAJoinRequest struct {
	ID             string `json:"id"`
	NodeName       string `json:"node_name"`
	NodeURL        string `json:"node_url"`
	NodeIP         string `json:"node_ip"`
	NodeMAC        string `json:"node_mac"`
	NodeHostname   string `json:"node_hostname"`
	NodeRole       string `json:"node_role"`
	NodeVersion    string `json:"node_version"`
	RequestedAt    string `json:"requested_at"`
	Status         string `json:"status"`
	RequesterToken string `json:"requester_token,omitempty"`
	HasToken       bool   `json:"has_token"`
}

type HAJoinRequestInput struct {
	NodeName       string `json:"node_name"`
	NodeURL        string `json:"node_url"`
	NodeIP         string `json:"node_ip"`
	NodeMAC        string `json:"node_mac"`
	NodeHostname   string `json:"node_hostname"`
	NodeRole       string `json:"node_role"`
	NodeVersion    string `json:"node_version"`
	RequesterToken string `json:"requester_token"`
}

type uniFiClient struct {
	IP       string
	Hostname string
	MAC      string
	Vendor   string
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

	secretKey, err := loadSecretKey(path)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, logger: logger, secretKey: secretKey}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func loadSecretKey(dbPath string) ([]byte, error) {
	keyPath := dbPath + ".key"
	if dir := filepath.Dir(keyPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	if data, err := os.ReadFile(keyPath); err == nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(decoded) != 32 {
			return nil, errors.New("invalid TM-DNS secret key file")
		}
		return decoded, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0600); err != nil {
		return nil, err
	}
	_ = os.Chmod(keyPath, 0600)
	return key, nil
}

func (s *Store) encryptSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *Store) decryptSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "enc:v1:") {
		return value
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "enc:v1:"))
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return ""
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return ""
	}
	return string(plain)
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
			vendor TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT 'Default',
			identity_confidence TEXT NOT NULL DEFAULT 'source_ip',
			identity_last_checked TEXT NOT NULL DEFAULT '',
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
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
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
	if err := s.ensureColumn(ctx, "hosts", "vendor", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "hosts", "identity_last_checked", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
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
	s.cacheMu.RLock()
	if s.staticLoaded {
		out := append([]StaticRecord(nil), s.staticCache...)
		s.cacheMu.RUnlock()
		return out, nil
	}
	s.cacheMu.RUnlock()
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	s.staticCache = append([]StaticRecord(nil), records...)
	s.staticLoaded = true
	s.cacheMu.Unlock()
	return records, nil
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
	if err == nil {
		s.invalidateStaticCache()
	}
	return err
}

func (s *Store) invalidateStaticCache() {
	s.cacheMu.Lock()
	s.staticLoaded = false
	s.staticCache = nil
	s.cacheMu.Unlock()
}

func (s *Store) Rules(ctx context.Context) ([]Rule, error) {
	s.cacheMu.RLock()
	if s.rulesLoaded {
		out := append([]Rule(nil), s.rulesCache...)
		s.cacheMu.RUnlock()
		return out, nil
	}
	s.cacheMu.RUnlock()
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	s.rulesCache = append([]Rule(nil), rules...)
	s.rulesLoaded = true
	s.cacheMu.Unlock()
	return rules, nil
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
	s.invalidateRuleCache()
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
	s.invalidateRuleCache()
	_ = s.Audit(ctx, "rule.enabled", fmt.Sprintf("%d", id), fmt.Sprintf("%t", enabled))
	return s.Rule(ctx, id)
}

func (s *Store) invalidateRuleCache() {
	s.cacheMu.Lock()
	s.rulesLoaded = false
	s.rulesCache = nil
	s.cacheMu.Unlock()
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
	cache, err := s.blocklistEntries(ctx)
	if err != nil {
		return nil, err
	}
	for candidate := qname; candidate != ""; candidate = parentDomain(candidate) {
		if match, ok := cache[candidate]; ok {
			return &match, nil
		}
	}
	return nil, nil
}

func (s *Store) blocklistEntries(ctx context.Context) (map[string]BlocklistMatch, error) {
	s.cacheMu.RLock()
	if s.blocklistLoaded {
		out := s.blocklistCache
		s.cacheMu.RUnlock()
		return out, nil
	}
	s.cacheMu.RUnlock()
	rows, err := s.db.QueryContext(ctx, `SELECT domain, source_name, source_type, source_id FROM blocklist_entries`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cache := map[string]BlocklistMatch{}
	for rows.Next() {
		var match BlocklistMatch
		if err := rows.Scan(&match.Domain, &match.SourceName, &match.SourceType, &match.SourceID); err != nil {
			return nil, err
		}
		cache[match.Domain] = match
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	s.blocklistCache = cache
	s.blocklistLoaded = true
	s.cacheMu.Unlock()
	return cache, nil
}

func (s *Store) invalidateBlocklistCache() {
	s.cacheMu.Lock()
	s.blocklistLoaded = false
	s.blocklistCache = nil
	s.cacheMu.Unlock()
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
	sources := []BlocklistSource{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, url, format, enabled, last_status, last_checked, created_at FROM blocklist_sources ORDER BY enabled DESC, name ASC`)
	if err != nil {
		return sources, err
	}
	defer rows.Close()
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
	if err := validatePublicHTTPURL(ctx, url); err != nil {
		return BlocklistSource{}, err
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
	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateBlocklistCache()
	return nil
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
		return
	}
	s.invalidateRuleCache()
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

type HostIdentityUpdate struct {
	Hostname   string
	MAC        string
	Vendor     string
	Confidence string
}

func (s *Store) EnrichHostIdentity(ctx context.Context, hostID int64, sourceIP string) error {
	sourceIP = strings.TrimSpace(sourceIP)
	if net.ParseIP(sourceIP) == nil || sourceIP == "unknown" {
		return nil
	}
	update := HostIdentityUpdate{}
	ptrCtx, ptrCancel := context.WithTimeout(ctx, 800*time.Millisecond)
	if hostname := lookupPTRHostname(ptrCtx, sourceIP); hostname != "" {
		update.Hostname = hostname
		update.Confidence = "ptr"
	}
	ptrCancel()

	arpCtx, arpCancel := context.WithTimeout(ctx, 800*time.Millisecond)
	arpHostname, mac := lookupARPIdentity(arpCtx, sourceIP)
	arpCancel()
	if update.Hostname == "" && arpHostname != "" {
		update.Hostname = arpHostname
		update.Confidence = "arp"
	}
	if mac != "" {
		update.MAC = mac
		update.Vendor = vendorFromMAC(mac)
		if update.Confidence == "" {
			update.Confidence = "arp"
		}
	}

	mdnsCtx, mdnsCancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	if hostname := lookupMDNSPTR(mdnsCtx, sourceIP); hostname != "" && update.Hostname == "" {
		update.Hostname = hostname
		update.Confidence = "mdns"
	}
	mdnsCancel()
	return s.UpdateHostIdentity(ctx, hostID, update)
}

func (s *Store) UpdateHostIdentity(ctx context.Context, hostID int64, update HostIdentityUpdate) error {
	update.Hostname = cleanHostname(update.Hostname)
	update.MAC = normalizeMAC(update.MAC)
	update.Vendor = strings.TrimSpace(update.Vendor)
	update.Confidence = strings.TrimSpace(update.Confidence)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentHostname, currentMAC, currentVendor, currentConfidence string
	if err := tx.QueryRowContext(ctx, `SELECT hostname, mac, vendor, identity_confidence FROM hosts WHERE id = ?`, hostID).
		Scan(&currentHostname, &currentMAC, &currentVendor, &currentConfidence); err != nil {
		return err
	}
	if update.Hostname == "" {
		update.Hostname = currentHostname
	}
	if update.MAC == "" {
		update.MAC = currentMAC
	}
	if update.Vendor == "" {
		update.Vendor = currentVendor
	}
	if update.Confidence == "" {
		update.Confidence = currentConfidence
	}

	_, err = tx.ExecContext(ctx, `UPDATE hosts
		SET hostname = CASE WHEN (hostname = '' OR ? = 'unifi') AND ? != '' THEN ? ELSE hostname END,
			mac = CASE WHEN (mac = '' OR ? = 'unifi') AND ? != '' THEN ? ELSE mac END,
			vendor = CASE WHEN (vendor = '' OR ? = 'unifi') AND ? != '' THEN ? ELSE vendor END,
			identity_confidence = CASE WHEN ? != '' AND (identity_confidence = 'source_ip' OR ? = 'unifi') THEN ? ELSE identity_confidence END,
			identity_last_checked = ?
		WHERE id = ?`,
		update.Confidence, update.Hostname, update.Hostname, update.Confidence, update.MAC, update.MAC, update.Confidence, update.Vendor, update.Vendor, update.Confidence, update.Confidence, update.Confidence, now, hostID)
	if err != nil {
		return err
	}
	for source, value := range map[string]string{
		"hostname": update.Hostname,
		"mac":      update.MAC,
		"vendor":   update.Vendor,
	} {
		if value == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO host_observations(host_id, source, value, observed_at) VALUES(?, ?, ?, ?)`, hostID, source, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
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
	empty := []QueryEvent{}
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
		return empty, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) Dashboard(ctx context.Context, dbPath string) (Dashboard, error) {
	var d Dashboard
	d.DatabasePath = dbPath
	d.EventStoreMode = "sqlite-wal"
	d.RetentionDays = s.RetentionDays(ctx)
	today := time.Now().Format("2006-01-02") + "T00:00:00"
	last48Hours := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM query_events WHERE ts >= ?`, today).Scan(&d.QueriesToday)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM query_events WHERE ts >= ? AND action = 'blocked'`, today).Scan(&d.BlockedToday)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT host_id) FROM query_events WHERE ts >= ?`, today).Scan(&d.UniqueHosts)
	d.Recent, _ = s.RecentEvents(ctx, "", 30)
	d.Blocked, _ = s.RecentEvents(ctx, "blocked", 30)
	d.TopHosts, _ = s.topHostRows(ctx, today)
	d.TopDomains, _ = s.topRowsWithPercent(ctx, `SELECT query_name, COUNT(*) FROM query_events WHERE ts >= ? GROUP BY query_name ORDER BY COUNT(*) DESC LIMIT 16`, `SELECT COUNT(*) FROM query_events WHERE ts >= ?`, last48Hours)
	d.RuleHits, _ = s.topRows(ctx, `SELECT target || ' (' || action || ')', hit_count FROM rules WHERE hit_count > 0 ORDER BY hit_count DESC LIMIT 8`)
	return d, nil
}

func (s *Store) Retention(ctx context.Context) (RetentionSettings, error) {
	settings := RetentionSettings{Days: s.RetentionDays(ctx)}
	_ = s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'retention.last_purge'`).Scan(&settings.LastPurge)
	return settings, nil
}

func (s *Store) HASettings(ctx context.Context) (HASettings, error) {
	settings := HASettings{Role: "primary"}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings WHERE key LIKE 'ha.%'`)
	if err != nil {
		return settings, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return settings, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return settings, err
	}
	settings.Enabled = values["ha.enabled"] == "true"
	if values["ha.role"] != "" {
		settings.Role = values["ha.role"]
	}
	settings.PeerName = values["ha.peer_name"]
	settings.PeerURL = values["ha.peer_url"]
	settings.PeerToken = s.decryptSecret(values["ha.peer_token"])
	settings.HasPeerToken = settings.PeerToken != ""
	settings.LastHeartbeat = values["ha.last_heartbeat"]
	settings.LastSync = values["ha.last_sync"]
	settings.LastStatus = values["ha.last_status"]
	return settings, nil
}

func (s *Store) SaveHASettings(ctx context.Context, settings HASettings) (HASettings, error) {
	settings.Role = strings.ToLower(strings.TrimSpace(settings.Role))
	settings.PeerName = strings.TrimSpace(settings.PeerName)
	settings.PeerURL = strings.TrimRight(strings.TrimSpace(settings.PeerURL), "/")
	settings.PeerToken = strings.TrimSpace(settings.PeerToken)
	if settings.Role == "" {
		settings.Role = "primary"
	}
	if settings.Role != "primary" && settings.Role != "secondary" {
		return HASettings{}, errors.New("role must be primary or secondary")
	}
	if settings.PeerURL != "" {
		parsed, err := url.Parse(settings.PeerURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return HASettings{}, errors.New("peer_url must be a full http:// or https:// URL")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HASettings{}, err
	}
	defer tx.Rollback()
	pairs := map[string]string{
		"ha.enabled":   fmt.Sprintf("%t", settings.Enabled),
		"ha.role":      settings.Role,
		"ha.peer_name": settings.PeerName,
		"ha.peer_url":  settings.PeerURL,
	}
	if settings.PeerToken != "" {
		encrypted, err := s.encryptSecret(settings.PeerToken)
		if err != nil {
			return HASettings{}, err
		}
		pairs["ha.peer_token"] = encrypted
	}
	for key, value := range pairs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return HASettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return HASettings{}, err
	}
	_ = s.Audit(ctx, "ha.settings", settings.PeerURL, fmt.Sprintf("enabled=%t role=%s", settings.Enabled, settings.Role))
	out, err := s.HASettings(ctx)
	if err != nil {
		return HASettings{}, err
	}
	out.PeerToken = ""
	return out, nil
}

func (s *Store) HAJoinRequests(ctx context.Context) ([]HAJoinRequest, error) {
	requests, err := s.haJoinRequestsRaw(ctx)
	if err != nil {
		return nil, err
	}
	for i := range requests {
		requests[i].RequesterToken = ""
		requests[i].HasToken = true
	}
	return requests, nil
}

func (s *Store) SaveHAJoinRequest(ctx context.Context, input HAJoinRequestInput) (HAJoinRequest, error) {
	input.NodeName = strings.TrimSpace(input.NodeName)
	input.NodeURL = strings.TrimRight(strings.TrimSpace(input.NodeURL), "/")
	input.NodeIP = strings.TrimSpace(input.NodeIP)
	input.NodeMAC = normalizeMAC(input.NodeMAC)
	input.NodeHostname = cleanHostname(input.NodeHostname)
	input.NodeRole = strings.ToLower(strings.TrimSpace(input.NodeRole))
	input.NodeVersion = strings.TrimSpace(input.NodeVersion)
	input.RequesterToken = strings.TrimSpace(input.RequesterToken)
	if input.NodeName == "" {
		input.NodeName = "TM-DNS Secondary"
	}
	if input.NodeRole == "" {
		input.NodeRole = "secondary"
	}
	if input.NodeRole != "secondary" {
		return HAJoinRequest{}, errors.New("join requests must come from a secondary node")
	}
	if input.NodeURL == "" {
		return HAJoinRequest{}, errors.New("node_url is required")
	}
	parsed, err := url.Parse(input.NodeURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return HAJoinRequest{}, errors.New("node_url must be a full http:// or https:// URL")
	}
	if input.RequesterToken == "" {
		return HAJoinRequest{}, errors.New("requester admin token is required")
	}
	requests, err := s.haJoinRequestsRaw(ctx)
	if err != nil {
		return HAJoinRequest{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	encrypted, err := s.encryptSecret(input.RequesterToken)
	if err != nil {
		return HAJoinRequest{}, err
	}
	request := HAJoinRequest{
		ID:             randomID(),
		NodeName:       input.NodeName,
		NodeURL:        input.NodeURL,
		NodeIP:         input.NodeIP,
		NodeMAC:        input.NodeMAC,
		NodeHostname:   input.NodeHostname,
		NodeRole:       input.NodeRole,
		NodeVersion:    input.NodeVersion,
		RequestedAt:    now,
		Status:         "pending",
		RequesterToken: encrypted,
	}
	replaced := false
	for i := range requests {
		if requests[i].NodeURL == input.NodeURL && requests[i].Status == "pending" {
			request.ID = requests[i].ID
			request.RequestedAt = now
			requests[i] = request
			replaced = true
			break
		}
	}
	if !replaced {
		requests = append(requests, request)
	}
	if err := s.saveHAJoinRequests(ctx, requests); err != nil {
		return HAJoinRequest{}, err
	}
	_ = s.Audit(ctx, "ha.join.request", input.NodeURL, input.NodeName)
	out := request
	out.RequesterToken = ""
	out.HasToken = true
	return out, nil
}

func (s *Store) AcceptHAJoinRequest(ctx context.Context, id string) (HAJoinRequest, error) {
	id = strings.TrimSpace(id)
	requests, err := s.haJoinRequestsRaw(ctx)
	if err != nil {
		return HAJoinRequest{}, err
	}
	for i := range requests {
		if requests[i].ID != id {
			continue
		}
		if requests[i].Status != "pending" {
			return HAJoinRequest{}, errors.New("join request is not pending")
		}
		token := s.decryptSecret(requests[i].RequesterToken)
		if token == "" {
			return HAJoinRequest{}, errors.New("join request token is unavailable")
		}
		if _, err := s.SaveHASettings(ctx, HASettings{
			Enabled:   true,
			Role:      "primary",
			PeerName:  requests[i].NodeName,
			PeerURL:   requests[i].NodeURL,
			PeerToken: token,
		}); err != nil {
			return HAJoinRequest{}, err
		}
		requests[i].Status = "accepted"
		if err := s.saveHAJoinRequests(ctx, requests); err != nil {
			return HAJoinRequest{}, err
		}
		_ = s.Audit(ctx, "ha.join.accept", requests[i].NodeURL, requests[i].NodeName)
		out := requests[i]
		out.RequesterToken = token
		out.HasToken = true
		return out, nil
	}
	return HAJoinRequest{}, errors.New("join request not found")
}

func (s *Store) haJoinRequestsRaw(ctx context.Context) ([]HAJoinRequest, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'ha.join_requests'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(raw) == "" {
		return []HAJoinRequest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var requests []HAJoinRequest
	if err := json.Unmarshal([]byte(raw), &requests); err != nil {
		return nil, err
	}
	return requests, nil
}

func (s *Store) saveHAJoinRequests(ctx context.Context, requests []HAJoinRequest) error {
	body, err := json.Marshal(requests)
	if err != nil {
		return err
	}
	return s.setSetting(ctx, "ha.join_requests", string(body))
}

func randomID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func (s *Store) TestHAPeer(ctx context.Context) (HAStatus, error) {
	settings, err := s.HASettings(ctx)
	if err != nil {
		return HAStatus{}, err
	}
	status := HAStatus{Status: "disabled", PeerURL: settings.PeerURL, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if !settings.Enabled {
		_ = s.setSetting(ctx, "ha.last_status", status.Status)
		return status, nil
	}
	if settings.PeerURL == "" {
		status.Status = "error"
		status.Error = "peer URL is required"
		_ = s.setSetting(ctx, "ha.last_status", status.Error)
		return status, nil
	}
	if settings.PeerToken == "" {
		status.Status = "error"
		status.Error = "peer token is required"
		_ = s.setSetting(ctx, "ha.last_status", status.Error)
		return status, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.PeerURL+"/api/ha/heartbeat", nil)
	if err != nil {
		return status, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.PeerToken)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		_ = s.setSetting(ctx, "ha.last_status", status.Error)
		return status, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		status.Status = "error"
		status.Error = resp.Status
		_ = s.setSetting(ctx, "ha.last_status", status.Error)
		return status, nil
	}
	var body struct {
		Version struct {
			Version string `json:"version"`
		} `json:"version"`
		HA HAHealth `json:"ha"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	status.Status = "ok"
	status.PeerVersion = body.Version.Version
	status.PeerRole = body.HA.Role
	_ = s.setSetting(ctx, "ha.last_heartbeat", status.CheckedAt)
	_ = s.setSetting(ctx, "ha.last_status", "ok")
	return status, nil
}

func (s *Store) ValidateHAImportAllowed(ctx context.Context) error {
	settings, err := s.HASettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return errors.New("secondary DNS sync is not enabled on this node")
	}
	if settings.Role != "secondary" {
		return errors.New("this node is not configured to receive HA policy sync")
	}
	return nil
}

func (s *Store) RetentionDays(ctx context.Context) int {
	var raw string
	_ = s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'retention.days'`).Scan(&raw)
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 1 {
		return 30
	}
	if parsed > 3650 {
		return 3650
	}
	return parsed
}

func (s *Store) SetRetention(ctx context.Context, days int) (RetentionSettings, error) {
	if days < 1 || days > 3650 {
		return RetentionSettings{}, errors.New("retention days must be between 1 and 3650")
	}
	if err := s.setSetting(ctx, "retention.days", strconv.Itoa(days)); err != nil {
		return RetentionSettings{}, err
	}
	_ = s.Audit(ctx, "retention.update", "query_events", fmt.Sprintf("%d days", days))
	return s.Retention(ctx)
}

func (s *Store) ExportHASyncPayload(ctx context.Context) (HASyncPayload, error) {
	records, err := s.StaticRecords(ctx)
	if err != nil {
		return HASyncPayload{}, err
	}
	rules, err := s.Rules(ctx)
	if err != nil {
		return HASyncPayload{}, err
	}
	presets, err := s.BlocklistPresets(ctx)
	if err != nil {
		return HASyncPayload{}, err
	}
	sources, err := s.BlocklistSources(ctx)
	if err != nil {
		return HASyncPayload{}, err
	}
	retention, err := s.Retention(ctx)
	if err != nil {
		return HASyncPayload{}, err
	}
	return HASyncPayload{
		StaticRecords:     records,
		Rules:             rules,
		BlocklistPresets:  presets,
		BlocklistSources:  sources,
		RetentionSettings: retention,
		SyncedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Store) ApplyHASyncPayload(ctx context.Context, payload HASyncPayload) (HASyncResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HASyncResult{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	recordKeys := map[string]struct{}{}
	for _, record := range payload.StaticRecords {
		name := NormalizeName(record.Name)
		recordType := strings.ToUpper(strings.TrimSpace(record.Type))
		if name == "" || recordType == "" || strings.TrimSpace(record.Value) == "" {
			continue
		}
		recordKeys[name+"\x00"+recordType] = struct{}{}
		if record.TTL <= 0 {
			record.TTL = 60
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO static_records(name, type, value, ttl, created_at)
			VALUES(?, ?, ?, ?, ?)
			ON CONFLICT(name, type) DO UPDATE SET value = excluded.value, ttl = excluded.ttl`,
			name, recordType, strings.TrimSpace(record.Value), record.TTL, now); err != nil {
			return HASyncResult{}, err
		}
	}
	if err := deleteMissingStaticRecords(ctx, tx, recordKeys); err != nil {
		return HASyncResult{}, err
	}
	ruleKeys := map[string]struct{}{}
	for _, rule := range payload.Rules {
		target := NormalizeName(rule.Target)
		action := strings.ToLower(strings.TrimSpace(rule.Action))
		if target == "" || (action != "block" && action != "allow") {
			continue
		}
		ruleKeys["global\x00"+target+"\x00"+action] = struct{}{}
		enabled := 0
		if rule.Enabled {
			enabled = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO rules(scope, target, action, enabled, note, created_at)
			VALUES('global', ?, ?, ?, ?, ?)
			ON CONFLICT(scope, target, action) DO UPDATE SET enabled = excluded.enabled, note = excluded.note`,
			target, action, enabled, rule.Note, now); err != nil {
			return HASyncResult{}, err
		}
	}
	if err := deleteMissingRules(ctx, tx, ruleKeys); err != nil {
		return HASyncResult{}, err
	}
	for _, preset := range payload.BlocklistPresets {
		if strings.TrimSpace(preset.ID) == "" {
			continue
		}
		enabled := 0
		if preset.Enabled {
			enabled = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO blocklist_presets(id, name, tier, description, home_url, source_url, enabled, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
			preset.ID, preset.Name, preset.Tier, preset.Description, preset.HomeURL, preset.SourceURL, enabled, now, now); err != nil {
			return HASyncResult{}, err
		}
	}
	sourceURLs := map[string]struct{}{}
	for _, source := range payload.BlocklistSources {
		source.URL = strings.TrimSpace(source.URL)
		if source.URL == "" {
			continue
		}
		if err := validateHAImportedBlocklistSource(source); err != nil {
			return HASyncResult{}, err
		}
		sourceURLs[source.URL] = struct{}{}
		enabled := 0
		if source.Enabled {
			enabled = 1
		}
		format := strings.ToLower(strings.TrimSpace(source.Format))
		if format == "" {
			format = "domains"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO blocklist_sources(name, url, format, enabled, created_at)
			VALUES(?, ?, ?, ?, ?)
			ON CONFLICT(url) DO UPDATE SET name = excluded.name, format = excluded.format, enabled = excluded.enabled`,
			source.Name, source.URL, format, enabled, now); err != nil {
			return HASyncResult{}, err
		}
	}
	if err := deleteMissingBlocklistSources(ctx, tx, sourceURLs); err != nil {
		return HASyncResult{}, err
	}
	if payload.RetentionSettings.Days > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES('retention.days', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.Itoa(payload.RetentionSettings.Days)); err != nil {
			return HASyncResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return HASyncResult{}, err
	}
	s.invalidateStaticCache()
	s.invalidateRuleCache()
	s.invalidateBlocklistCache()
	_ = s.setSetting(ctx, "ha.last_sync", now)
	_ = s.setSetting(ctx, "ha.last_status", "sync received")
	_ = s.Audit(ctx, "ha.sync.receive", "peer", fmt.Sprintf("records=%d rules=%d sources=%d", len(payload.StaticRecords), len(payload.Rules), len(payload.BlocklistSources)))
	return HASyncResult{Status: "ok", StaticRecords: len(payload.StaticRecords), Rules: len(payload.Rules), BlocklistPresets: len(payload.BlocklistPresets), BlocklistSources: len(payload.BlocklistSources)}, nil
}

func deleteMissingStaticRecords(ctx context.Context, tx *sql.Tx, keep map[string]struct{}) error {
	rows, err := tx.QueryContext(ctx, `SELECT name, type FROM static_records`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type key struct {
		name       string
		recordType string
	}
	var remove []key
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.name, &item.recordType); err != nil {
			return err
		}
		if _, ok := keep[item.name+"\x00"+item.recordType]; !ok {
			remove = append(remove, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM static_records WHERE name = ? AND type = ?`, item.name, item.recordType); err != nil {
			return err
		}
	}
	return nil
}

func deleteMissingRules(ctx context.Context, tx *sql.Tx, keep map[string]struct{}) error {
	rows, err := tx.QueryContext(ctx, `SELECT scope, target, action FROM rules`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type key struct {
		scope  string
		target string
		action string
	}
	var remove []key
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.scope, &item.target, &item.action); err != nil {
			return err
		}
		if _, ok := keep[item.scope+"\x00"+item.target+"\x00"+item.action]; !ok {
			remove = append(remove, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM rules WHERE scope = ? AND target = ? AND action = ?`, item.scope, item.target, item.action); err != nil {
			return err
		}
	}
	return nil
}

func deleteMissingBlocklistSources(ctx context.Context, tx *sql.Tx, keep map[string]struct{}) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, url FROM blocklist_sources`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type source struct {
		id  int64
		url string
	}
	var remove []source
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.id, &item.url); err != nil {
			return err
		}
		if _, ok := keep[item.url]; !ok {
			remove = append(remove, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM blocklist_entries WHERE source_type = 'custom' AND source_id = ?`, fmt.Sprintf("%d", item.id)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM blocklist_sources WHERE id = ?`, item.id); err != nil {
			return err
		}
	}
	return nil
}

func validateHAImportedBlocklistSource(source BlocklistSource) error {
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return errors.New("imported blocklist source has invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("imported blocklist source url must use http or https")
	}
	format := strings.ToLower(strings.TrimSpace(source.Format))
	if format != "" && format != "domains" && format != "hosts" && format != "adguard" {
		return errors.New("imported blocklist source format must be domains, hosts, or adguard")
	}
	return nil
}

func (s *Store) PushHASync(ctx context.Context) (HASyncResult, error) {
	return s.pushHASync(ctx, true)
}

func (s *Store) pushHASync(ctx context.Context, audit bool) (HASyncResult, error) {
	settings, err := s.HASettings(ctx)
	if err != nil {
		return HASyncResult{}, err
	}
	result := HASyncResult{Status: "disabled", PeerURL: settings.PeerURL}
	if !settings.Enabled {
		return result, nil
	}
	if settings.PeerURL == "" || settings.PeerToken == "" {
		result.Status = "error"
		result.Error = "peer URL and token are required"
		_ = s.setSetting(ctx, "ha.last_status", result.Error)
		return result, nil
	}
	payload, err := s.ExportHASyncPayload(ctx)
	if err != nil {
		return HASyncResult{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return HASyncResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.PeerURL+"/api/ha/import", strings.NewReader(string(body)))
	if err != nil {
		return HASyncResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.PeerToken)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		_ = s.setSetting(ctx, "ha.last_status", result.Error)
		return result, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		result.Status = "error"
		result.Error = resp.Status
		_ = s.setSetting(ctx, "ha.last_status", result.Error)
		return result, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return HASyncResult{}, err
	}
	result.PeerURL = settings.PeerURL
	_ = s.setSetting(ctx, "ha.last_sync", time.Now().UTC().Format(time.RFC3339Nano))
	_ = s.setSetting(ctx, "ha.last_status", "sync pushed")
	if audit {
		_ = s.Audit(ctx, "ha.sync.push", settings.PeerURL, fmt.Sprintf("records=%d rules=%d sources=%d", result.StaticRecords, result.Rules, result.BlocklistSources))
	}
	return result, nil
}

func (s *Store) HAHealth(ctx context.Context) (HAHealth, error) {
	settings, err := s.HASettings(ctx)
	if err != nil {
		return HAHealth{}, err
	}
	health := HAHealth{
		Enabled:       settings.Enabled,
		Role:          settings.Role,
		PeerName:      settings.PeerName,
		PeerURL:       settings.PeerURL,
		Status:        settings.LastStatus,
		LastHeartbeat: settings.LastHeartbeat,
		LastSync:      settings.LastSync,
		Configured:    settings.PeerURL != "" && settings.HasPeerToken,
	}
	if health.Status == "" {
		health.Status = "not checked"
	}
	if settings.LastHeartbeat != "" {
		if ts, err := time.Parse(time.RFC3339Nano, settings.LastHeartbeat); err == nil {
			health.HeartbeatAgeSeconds = int64(time.Since(ts).Seconds())
			health.Stale = settings.Enabled && health.HeartbeatAgeSeconds > 180
		}
	} else if settings.Enabled {
		health.Stale = true
	}
	return health, nil
}

func (s *Store) StartHAWorker(ctx context.Context, logger *slog.Logger) {
	go func() {
		heartbeatTicker := time.NewTicker(1 * time.Minute)
		syncTicker := time.NewTicker(5 * time.Minute)
		defer heartbeatTicker.Stop()
		defer syncTicker.Stop()

		runHeartbeat := func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			settings, err := s.HASettings(runCtx)
			if err != nil {
				logger.Warn("ha settings load failed", "error", err)
				return
			}
			if !settings.Enabled || settings.PeerURL == "" {
				return
			}
			status, err := s.TestHAPeer(runCtx)
			if err != nil {
				logger.Warn("ha heartbeat failed", "error", err)
				return
			}
			if status.Status != "ok" {
				logger.Warn("ha heartbeat unhealthy", "peer", status.PeerURL, "status", status.Status, "error", status.Error)
				return
			}
			logger.Debug("ha heartbeat ok", "peer", status.PeerURL, "version", status.PeerVersion)
		}

		runSync := func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			settings, err := s.HASettings(runCtx)
			if err != nil {
				logger.Warn("ha settings load failed", "error", err)
				return
			}
			if !settings.Enabled || settings.Role != "primary" || settings.PeerURL == "" {
				return
			}
			result, err := s.pushHASync(runCtx, false)
			if err != nil {
				logger.Warn("ha sync failed", "error", err)
				return
			}
			if result.Status != "ok" {
				logger.Warn("ha sync unhealthy", "peer", result.PeerURL, "status", result.Status, "error", result.Error)
				return
			}
			logger.Debug("ha sync ok", "peer", result.PeerURL, "rules", result.Rules, "records", result.StaticRecords, "sources", result.BlocklistSources)
		}

		runHeartbeat()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				runHeartbeat()
			case <-syncTicker.C:
				runSync()
			}
		}
	}()
}

func (s *Store) PurgeOldEvents(ctx context.Context) (int64, error) {
	days := s.RetentionDays(ctx)
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM query_events WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM block_events WHERE created_at < ?`, cutoff); err != nil {
		return 0, err
	}
	count, _ := res.RowsAffected()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES('retention.last_purge', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, now); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if count > 0 {
		_ = s.Audit(ctx, "retention.purge", "query_events", fmt.Sprintf("%d removed before %s", count, cutoff))
	}
	return count, nil
}

func (s *Store) Hosts(ctx context.Context) ([]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, source_ip, label, hostname, mac, vendor, group_name, identity_confidence, identity_last_checked, first_seen, last_seen, query_count, block_count, notes FROM hosts ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []Host
	for rows.Next() {
		var host Host
		if err := rows.Scan(&host.ID, &host.SourceIP, &host.Label, &host.Hostname, &host.MAC, &host.Vendor, &host.Group, &host.IdentityConfidence, &host.IdentityLastChecked, &host.FirstSeen, &host.LastSeen, &host.QueryCount, &host.BlockCount, &host.Notes); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, rows.Err()
}

func (s *Store) Host(ctx context.Context, id int64) (Host, error) {
	var host Host
	err := s.db.QueryRowContext(ctx, `SELECT id, source_ip, label, hostname, mac, vendor, group_name, identity_confidence, identity_last_checked, first_seen, last_seen, query_count, block_count, notes FROM hosts WHERE id = ?`, id).
		Scan(&host.ID, &host.SourceIP, &host.Label, &host.Hostname, &host.MAC, &host.Vendor, &host.Group, &host.IdentityConfidence, &host.IdentityLastChecked, &host.FirstSeen, &host.LastSeen, &host.QueryCount, &host.BlockCount, &host.Notes)
	return host, err
}

func (s *Store) HostDetail(ctx context.Context, id int64, hours int) (HostDetail, error) {
	host, err := s.Host(ctx, id)
	if err != nil {
		return HostDetail{}, err
	}
	if hours != 48 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339Nano)
	recent, err := s.RecentEventsByHostSince(ctx, id, "", since, 1000)
	if err != nil {
		return HostDetail{}, err
	}
	blocked, err := s.RecentEventsByHostSince(ctx, id, "blocked", since, 1000)
	if err != nil {
		return HostDetail{}, err
	}
	topDomains, _ := s.topRows(ctx, `SELECT query_name, COUNT(*) FROM query_events WHERE host_id = ? AND ts >= ? GROUP BY query_name ORDER BY COUNT(*) DESC LIMIT 50`, id, since)
	topActions, _ := s.topRows(ctx, `SELECT action, COUNT(*) FROM query_events WHERE host_id = ? AND ts >= ? GROUP BY action ORDER BY COUNT(*) DESC`, id, since)
	var totalQueries, totalBlocked, uniqueDomains int64
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT query_name) FROM query_events WHERE host_id = ? AND ts >= ?`, id, since).Scan(&totalQueries, &uniqueDomains)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM query_events WHERE host_id = ? AND ts >= ? AND action = 'blocked'`, id, since).Scan(&totalBlocked)
	return HostDetail{Host: host, WindowHours: hours, TotalQueries: totalQueries, TotalBlocked: totalBlocked, UniqueDomains: uniqueDomains, Recent: recent, Blocked: blocked, TopDomains: topDomains, TopActions: topActions}, nil
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
	return s.RecentEventsByHostSince(ctx, hostID, action, "", limit)
}

func (s *Store) RecentEventsByHostSince(ctx context.Context, hostID int64, action, since string, limit int) ([]QueryEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	where := "WHERE qe.host_id = ?"
	args := []any{hostID}
	if since != "" {
		where += " AND qe.ts >= ?"
		args = append(args, since)
	}
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
	events := []AuditEvent{}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, action, target, detail FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return events, err
	}
	defer rows.Close()
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.Action, &event.Target, &event.Detail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) UniFiSettings(ctx context.Context) (UniFiSettings, error) {
	settings := UniFiSettings{Site: "default"}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings WHERE key LIKE 'unifi.%'`)
	if err != nil {
		return settings, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return settings, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return settings, err
	}
	settings.Enabled = values["unifi.enabled"] == "true"
	settings.BaseURL = values["unifi.base_url"]
	if values["unifi.site"] != "" {
		settings.Site = values["unifi.site"]
	}
	settings.APIKey = s.decryptSecret(values["unifi.api_key"])
	settings.HasAPIKey = settings.APIKey != ""
	settings.LastImport = values["unifi.last_import"]
	settings.LastStatus = values["unifi.last_status"]
	return settings, nil
}

func (s *Store) SaveUniFiSettings(ctx context.Context, settings UniFiSettings) (UniFiSettings, error) {
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	settings.Site = strings.TrimSpace(settings.Site)
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	if settings.Site == "" {
		settings.Site = "default"
	}
	if settings.BaseURL != "" {
		parsed, err := url.Parse(settings.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return UniFiSettings{}, errors.New("base_url must be a full http:// or https:// URL")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UniFiSettings{}, err
	}
	defer tx.Rollback()
	pairs := map[string]string{
		"unifi.enabled":  fmt.Sprintf("%t", settings.Enabled),
		"unifi.base_url": settings.BaseURL,
		"unifi.site":     settings.Site,
	}
	if settings.APIKey != "" {
		encrypted, err := s.encryptSecret(settings.APIKey)
		if err != nil {
			return UniFiSettings{}, err
		}
		pairs["unifi.api_key"] = encrypted
	}
	for key, value := range pairs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return UniFiSettings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return UniFiSettings{}, err
	}
	_ = s.Audit(ctx, "unifi.settings", settings.BaseURL, fmt.Sprintf("enabled=%t site=%s", settings.Enabled, settings.Site))
	out, err := s.UniFiSettings(ctx)
	if err != nil {
		return UniFiSettings{}, err
	}
	out.APIKey = ""
	return out, nil
}

func (s *Store) TestUniFi(ctx context.Context) (UniFiImportResult, error) {
	settings, err := s.UniFiSettings(ctx)
	if err != nil {
		return UniFiImportResult{}, err
	}
	clients, err := fetchUniFiClients(ctx, settings)
	result := UniFiImportResult{Status: "ok", Seen: len(clients)}
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
	}
	_ = s.setSetting(ctx, "unifi.last_status", result.Status)
	return result, err
}

func (s *Store) ImportUniFiClients(ctx context.Context) (UniFiImportResult, error) {
	settings, err := s.UniFiSettings(ctx)
	if err != nil {
		return UniFiImportResult{}, err
	}
	if !settings.Enabled {
		return UniFiImportResult{Status: "disabled", Error: "UniFi import is disabled"}, nil
	}
	clients, err := fetchUniFiClients(ctx, settings)
	if err != nil {
		_ = s.setSetting(ctx, "unifi.last_status", "error: "+err.Error())
		return UniFiImportResult{Status: "error", Error: err.Error()}, err
	}
	updated := 0
	for _, client := range clients {
		if net.ParseIP(client.IP) == nil {
			continue
		}
		hostID, _, err := s.EnsureHost(ctx, client.IP)
		if err != nil {
			return UniFiImportResult{}, err
		}
		if err := s.UpdateHostIdentity(ctx, hostID, HostIdentityUpdate{
			Hostname:   client.Hostname,
			MAC:        client.MAC,
			Vendor:     client.Vendor,
			Confidence: "unifi",
		}); err != nil {
			return UniFiImportResult{}, err
		}
		updated++
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = s.setSetting(ctx, "unifi.last_import", now)
	_ = s.setSetting(ctx, "unifi.last_status", "ok")
	_ = s.Audit(ctx, "unifi.import", settings.BaseURL, fmt.Sprintf("seen=%d updated=%d", len(clients), updated))
	return UniFiImportResult{Status: "ok", Seen: len(clients), Updated: updated}, nil
}

func (s *Store) setSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
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

var arpMACRE = regexp.MustCompile(`(?i)\b([0-9a-f]{1,2}(?::[0-9a-f]{1,2}){5})\b`)

func lookupPTRHostname(ctx context.Context, sourceIP string) string {
	names, err := net.DefaultResolver.LookupAddr(ctx, sourceIP)
	if err != nil {
		return ""
	}
	for _, name := range names {
		if cleaned := cleanHostname(name); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func lookupMDNSPTR(ctx context.Context, sourceIP string) string {
	reverse := reversePTRName(sourceIP)
	if reverse == "" {
		return ""
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/dns-sd", "-Q", reverse, "PTR")
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}
	return parseDNSSDPTR(output)
}

func lookupARPIdentity(ctx context.Context, sourceIP string) (string, string) {
	cmd := exec.CommandContext(ctx, "/usr/sbin/arp", "-n", sourceIP)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		cmd = exec.CommandContext(ctx, "/usr/sbin/arp", sourceIP)
		output, err = cmd.CombinedOutput()
		if err != nil && len(output) == 0 {
			return "", ""
		}
	}
	return parseARPOutput(string(output), sourceIP)
}

func parseARPOutput(output, sourceIP string) (string, string) {
	var hostname string
	for _, line := range strings.Split(output, "\n") {
		if sourceIP != "" && !strings.Contains(line, sourceIP) {
			continue
		}
		mac := ""
		if match := arpMACRE.FindStringSubmatch(line); len(match) > 1 {
			mac = normalizeMAC(match[1])
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] != "?" && fields[0] != sourceIP {
			hostname = cleanHostname(fields[0])
		}
		if mac != "" {
			return hostname, mac
		}
	}
	return hostname, ""
}

func reversePTRName(sourceIP string) string {
	ip := net.ParseIP(sourceIP)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", v4[3], v4[2], v4[1], v4[0])
	}
	return ""
}

func parseDNSSDPTR(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		for i := len(fields) - 1; i >= 0; i-- {
			if strings.HasSuffix(strings.ToLower(fields[i]), ".local.") {
				return cleanHostname(fields[i])
			}
		}
	}
	return ""
}

func cleanHostname(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".")
	if value == "" || value == "?" || strings.EqualFold(value, "unknown") {
		return ""
	}
	return strings.ToLower(value)
}

func normalizeMAC(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ":")
	if len(parts) != 6 {
		return ""
	}
	for i, part := range parts {
		if len(part) == 1 {
			parts[i] = "0" + part
		}
		if len(parts[i]) != 2 {
			return ""
		}
	}
	return strings.Join(parts, ":")
}

func vendorFromMAC(mac string) string {
	prefix := strings.ToUpper(strings.ReplaceAll(normalizeMAC(mac), ":", ""))
	if len(prefix) < 6 {
		return ""
	}
	switch prefix[:6] {
	case "00163E":
		return "Xensource"
	case "001C42":
		return "Parallels"
	case "005056":
		return "VMware"
	case "00E04C":
		return "Realtek"
	case "D011E5", "3C22FB", "F0D1A9", "A8A159", "ACDE48":
		return "Apple"
	case "60F81D", "68D79A", "70A741", "7483C2", "B4FBE4", "D021F9", "E063DA":
		return "Ubiquiti"
	default:
		return ""
	}
}

func fetchUniFiClients(ctx context.Context, settings UniFiSettings) ([]uniFiClient, error) {
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	settings.Site = strings.TrimSpace(settings.Site)
	if settings.Site == "" {
		settings.Site = "default"
	}
	if settings.BaseURL == "" {
		return nil, errors.New("UniFi base URL is required")
	}
	if settings.APIKey == "" {
		return nil, errors.New("UniFi API key is required")
	}
	endpoints := []string{
		fmt.Sprintf("%s/proxy/network/api/s/%s/stat/sta", settings.BaseURL, url.PathEscape(settings.Site)),
		fmt.Sprintf("%s/proxy/network/api/s/%s/rest/user", settings.BaseURL, url.PathEscape(settings.Site)),
		fmt.Sprintf("%s/api/s/%s/stat/sta", settings.BaseURL, url.PathEscape(settings.Site)),
		fmt.Sprintf("%s/api/s/%s/rest/user", settings.BaseURL, url.PathEscape(settings.Site)),
	}
	client := http.Client{Timeout: 8 * time.Second}
	var lastErr error
	for _, endpoint := range endpoints {
		clients, err := fetchUniFiEndpoint(ctx, &client, endpoint, settings.APIKey)
		if err == nil {
			return clients, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no UniFi endpoints attempted")
	}
	return nil, lastErr
}

func fetchUniFiEndpoint(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]uniFiClient, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("UniFi returned HTTP %d for %s", resp.StatusCode, endpoint)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(&body); err != nil {
		return nil, err
	}
	clients := make([]uniFiClient, 0, len(body.Data))
	for _, raw := range body.Data {
		client := uniFiClient{
			IP:       firstString(raw, "ip", "last_ip", "fixed_ip"),
			Hostname: firstString(raw, "hostname", "name", "display_name", "dev_alias"),
			MAC:      firstString(raw, "mac"),
			Vendor:   firstString(raw, "oui", "vendor"),
		}
		client.Hostname = cleanHostname(client.Hostname)
		client.MAC = normalizeMAC(client.MAC)
		if client.Vendor == "" {
			client.Vendor = vendorFromMAC(client.MAC)
		}
		if client.IP != "" && (client.Hostname != "" || client.MAC != "") {
			clients = append(clients, client)
		}
	}
	if len(clients) == 0 {
		return nil, errors.New("UniFi request succeeded but returned no clients with IP/name data")
	}
	return clients, nil
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return fmt.Sprintf("%.0f", typed)
		}
	}
	return ""
}

func validatePublicHTTPURL(ctx context.Context, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return errors.New("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	host := parsed.Hostname()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if isPrivateFetchIP(ip.IP) {
			return errors.New("custom blocklist URL must resolve to a public address")
		}
	}
	return nil
}

func isPrivateFetchIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func (s *Store) topRows(ctx context.Context, query string, args ...any) ([]TopRow, error) {
	out := []TopRow{}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var row TopRow
		if err := rows.Scan(&row.Key, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) topRowsWithPercent(ctx context.Context, rowsQuery, totalQuery string, args ...any) ([]TopRow, error) {
	out, err := s.topRows(ctx, rowsQuery, args...)
	if err != nil {
		return out, err
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, totalQuery, args...).Scan(&total); err != nil || total <= 0 {
		return out, err
	}
	for i := range out {
		out[i].Percent = (float64(out[i].Count) / float64(total)) * 100
	}
	return out, nil
}

func (s *Store) topHostRows(ctx context.Context, since string) ([]TopHostRow, error) {
	out := []TopHostRow{}
	rows, err := s.db.QueryContext(ctx, `SELECT h.id, COALESCE(NULLIF(h.label, ''), NULLIF(h.hostname, ''), h.source_ip), h.source_ip, h.label, h.hostname, h.mac, h.vendor, COUNT(*)
		FROM query_events qe JOIN hosts h ON h.id = qe.host_id
		WHERE qe.ts >= ?
		GROUP BY qe.host_id
		ORDER BY COUNT(*) DESC
		LIMIT 8`, since)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var row TopHostRow
		if err := rows.Scan(&row.ID, &row.Key, &row.SourceIP, &row.Label, &row.Hostname, &row.MAC, &row.Vendor, &row.Count); err != nil {
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
	events := []QueryEvent{}
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
