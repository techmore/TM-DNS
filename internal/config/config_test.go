package config

import "testing"

func TestResolveBindAddrKeepsExplicitAddress(t *testing.T) {
	got := ResolveBindAddr("127.0.0.1:1053")
	if got != "127.0.0.1:1053" {
		t.Fatalf("ResolveBindAddr explicit = %q", got)
	}
}

func TestResolveHTTPBindAddrAutoBindsAllInterfaces(t *testing.T) {
	got := ResolveHTTPBindAddr("auto:8080")
	if got != "0.0.0.0:8080" {
		t.Fatalf("ResolveHTTPBindAddr auto = %q", got)
	}
}

func TestResolveBindAddrAutoBindsAllInterfaces(t *testing.T) {
	got := ResolveBindAddr("auto:53")
	if got != "0.0.0.0:53" {
		t.Fatalf("ResolveBindAddr auto = %q", got)
	}
}

func TestParseNetworkSetupHardwarePorts(t *testing.T) {
	ports := parseNetworkSetupHardwarePorts(`Hardware Port: Ethernet
Device: en0
Ethernet Address: aa:bb:cc:dd:ee:ff

Hardware Port: Wi-Fi
Device: en1
Ethernet Address: aa:bb:cc:dd:ee:00
`)
	if ports["en0"] != "Ethernet" {
		t.Fatalf("en0 port = %q", ports["en0"])
	}
	if ports["en1"] != "Wi-Fi" {
		t.Fatalf("en1 port = %q", ports["en1"])
	}
}

func TestInterfaceScorePrefersWiredOverWiFi(t *testing.T) {
	wired := interfaceScore("en13", "USB 10/100/1G/2.5G LAN", "192.168.222.8")
	wifi := interfaceScore("en1", "Wi-Fi", "192.168.222.9")
	if wired <= wifi {
		t.Fatalf("wired score %d should beat wifi score %d", wired, wifi)
	}
}
