package dnsserver

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/techmore/tm-dns/internal/config"
	"github.com/techmore/tm-dns/internal/store"
)

func TestServerAnswersStaticRecordOverUDP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testStore(t)
	defer st.Close()
	if err := st.UpsertStaticRecord(ctx, store.StaticRecord{Name: "router.test", Type: "A", Value: "192.168.1.1", TTL: 60}); err != nil {
		t.Fatal(err)
	}

	addr := startTestServer(t, ctx, st)
	msg := query(t, addr, "router.test.", dns.TypeA)
	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(msg.Answer))
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok || !a.A.Equal(net.ParseIP("192.168.1.1")) {
		t.Fatalf("answer = %#v", msg.Answer[0])
	}
}

func TestServerSinkholesBlockedRuleOverUDP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testStore(t)
	defer st.Close()
	if _, err := st.AddRule(ctx, "blocked.test", "block", "test"); err != nil {
		t.Fatal(err)
	}

	addr := startTestServer(t, ctx, st)
	msg := query(t, addr, "blocked.test.", dns.TypeA)
	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(msg.Answer))
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok || !a.A.Equal(net.ParseIP("0.0.0.0")) {
		t.Fatalf("answer = %#v", msg.Answer[0])
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir()+"/test.db", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func startTestServer(t *testing.T, ctx context.Context, st *store.Store) string {
	t.Helper()
	addr := freeUDPAddr(t)
	cfg := config.Config{
		DNSAddr:       addr,
		DBPath:        t.TempDir() + "/test.db",
		Upstream:      "127.0.0.1:9",
		SinkholeIPv4:  "0.0.0.0",
		SinkholeIPv6:  "::",
		QueryTimeout:  250 * time.Millisecond,
		EventQueueCap: 100,
	}
	srv := New(cfg, st, slog.Default())
	errs := make(chan error, 1)
	go func() {
		errs <- srv.Start(ctx)
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown()
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errs:
			t.Fatalf("server exited: %v", err)
		default:
		}
		c := &dns.Client{Net: "udp", Timeout: 100 * time.Millisecond}
		m := new(dns.Msg)
		m.SetQuestion("health.test.", dns.TypeA)
		_, _, err := c.Exchange(m, addr)
		if err == nil || time.Now().After(deadline.Add(-1500*time.Millisecond)) {
			return addr
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return ""
}

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	c := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	return pc.LocalAddr().String()
}
