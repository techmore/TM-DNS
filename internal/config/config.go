package config

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DNSAddr        string
	HTTPAddr       string
	DBPath         string
	AdminToken     string
	AdminTokenPath string
	Upstream       string
	SinkholeIPv4   string
	SinkholeIPv6   string
	LogLevel       string
	QueryTimeout   time.Duration
	EventQueueCap  int
}

func Load() Config {
	dnsAddr := getenv("TMDNS_DNS_ADDR", "auto:53")
	httpAddr := getenv("TMDNS_HTTP_ADDR", "auto:8080")
	return Config{
		DNSAddr:        ResolveBindAddr(dnsAddr),
		HTTPAddr:       ResolveHTTPBindAddr(httpAddr),
		DBPath:         getenv("TMDNS_DB_PATH", "tm-dns.db"),
		AdminToken:     strings.TrimSpace(os.Getenv("TMDNS_ADMIN_TOKEN")),
		AdminTokenPath: getenv("TMDNS_ADMIN_TOKEN_PATH", ""),
		Upstream:       getenv("TMDNS_UPSTREAM", "1.1.1.1:53"),
		SinkholeIPv4:   getenv("TMDNS_SINKHOLE_IPV4", "0.0.0.0"),
		SinkholeIPv6:   getenv("TMDNS_SINKHOLE_IPV6", "::"),
		LogLevel:       strings.ToLower(getenv("TMDNS_LOG_LEVEL", "debug")),
		QueryTimeout:   time.Duration(getenvInt("TMDNS_QUERY_TIMEOUT_MS", 2500)) * time.Millisecond,
		EventQueueCap:  getenvInt("TMDNS_EVENT_QUEUE_CAP", 10000),
	}
}

func ResolveHTTPBindAddr(value string) string {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return value
	}
	if host != "auto" && host != "lan" {
		return value
	}
	return net.JoinHostPort("0.0.0.0", port)
}

func ResolveBindAddr(value string) string {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return value
	}
	if host != "auto" && host != "lan" {
		return value
	}
	return net.JoinHostPort("0.0.0.0", port)
}

type interfaceCandidate struct {
	Name         string
	HardwarePort string
	IP           string
	Score        int
}

func AutoLANIPv4() (string, bool) {
	hardwarePorts := macHardwarePorts()
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}
	var best interfaceCandidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipv4FromAddr(addr)
			if ip == "" || !isUsableLANIPv4(ip) {
				continue
			}
			candidate := interfaceCandidate{
				Name:         iface.Name,
				HardwarePort: hardwarePorts[iface.Name],
				IP:           ip,
				Score:        interfaceScore(iface.Name, hardwarePorts[iface.Name], ip),
			}
			if candidate.Score > best.Score {
				best = candidate
			}
		}
	}
	return best.IP, best.IP != ""
}

func ipv4FromAddr(addr net.Addr) string {
	var ip net.IP
	switch value := addr.(type) {
	case *net.IPNet:
		ip = value.IP
	case *net.IPAddr:
		ip = value.IP
	default:
		return ""
	}
	ip = ip.To4()
	if ip == nil {
		return ""
	}
	return ip.String()
}

func isUsableLANIPv4(value string) bool {
	ip := net.ParseIP(value).To4()
	if ip == nil {
		return false
	}
	if ip[0] == 127 || ip[0] == 169 {
		return false
	}
	return true
}

func interfaceScore(name, hardwarePort, ip string) int {
	lowerName := strings.ToLower(name)
	lowerPort := strings.ToLower(hardwarePort)
	score := 10
	switch {
	case strings.Contains(lowerPort, "wi-fi") || strings.Contains(lowerPort, "wifi") || strings.Contains(lowerPort, "airport"):
		score += 20
	case strings.Contains(lowerPort, "ethernet") || strings.Contains(lowerPort, "lan") || strings.Contains(lowerPort, "thunderbolt"):
		score += 90
	case strings.HasPrefix(lowerName, "en"):
		score += 45
	}
	if strings.Contains(lowerName, "bridge") || strings.Contains(lowerName, "awdl") || strings.Contains(lowerName, "llw") || strings.Contains(lowerName, "utun") {
		score -= 100
	}
	if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") || private172(ip) {
		score += 10
	}
	return score
}

func private172(ip string) bool {
	parsed := net.ParseIP(ip).To4()
	return parsed != nil && parsed[0] == 172 && parsed[1] >= 16 && parsed[1] <= 31
}

func macHardwarePorts() map[string]string {
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return map[string]string{}
	}
	return parseNetworkSetupHardwarePorts(string(out))
}

func parseNetworkSetupHardwarePorts(output string) map[string]string {
	ports := map[string]string{}
	var current string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Hardware Port:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
		case strings.HasPrefix(line, "Device:"):
			device := strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
			if current != "" && device != "" {
				ports[device] = current
			}
		}
	}
	return ports
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
