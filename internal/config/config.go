package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DNSAddr       string
	HTTPAddr      string
	DBPath        string
	Upstream      string
	SinkholeIPv4  string
	SinkholeIPv6  string
	LogLevel      string
	QueryTimeout  time.Duration
	EventQueueCap int
}

func Load() Config {
	return Config{
		DNSAddr:       getenv("TMDNS_DNS_ADDR", "127.0.0.1:1053"),
		HTTPAddr:      getenv("TMDNS_HTTP_ADDR", "127.0.0.1:8080"),
		DBPath:        getenv("TMDNS_DB_PATH", "tm-dns.db"),
		Upstream:      getenv("TMDNS_UPSTREAM", "1.1.1.1:53"),
		SinkholeIPv4:  getenv("TMDNS_SINKHOLE_IPV4", "0.0.0.0"),
		SinkholeIPv6:  getenv("TMDNS_SINKHOLE_IPV6", "::"),
		LogLevel:      strings.ToLower(getenv("TMDNS_LOG_LEVEL", "debug")),
		QueryTimeout:  time.Duration(getenvInt("TMDNS_QUERY_TIMEOUT_MS", 2500)) * time.Millisecond,
		EventQueueCap: getenvInt("TMDNS_EVENT_QUEUE_CAP", 10000),
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
