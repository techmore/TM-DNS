package dnsserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"github.com/techmore/tm-dns/internal/config"
	"github.com/techmore/tm-dns/internal/store"
)

type Server struct {
	cfg       config.Config
	store     *store.Store
	logger    *slog.Logger
	udp       *dns.Server
	tcp       *dns.Server
	events    chan store.QueryEvent
	startedAt time.Time
	dropped   atomic.Int64
	queries   atomic.Int64
	blocked   atomic.Int64
	mu        sync.RWMutex
}

type Status struct {
	StartedAt       time.Time `json:"started_at"`
	UptimeSeconds   int64     `json:"uptime_seconds"`
	Queries         int64     `json:"queries"`
	Blocked         int64     `json:"blocked"`
	DroppedEvents   int64     `json:"dropped_events"`
	DNSAddr         string    `json:"dns_addr"`
	Upstream        string    `json:"upstream"`
	EventQueueDepth int       `json:"event_queue_depth"`
}

func New(cfg config.Config, st *store.Store, logger *slog.Logger) *Server {
	return &Server{
		cfg:       cfg,
		store:     st,
		logger:    logger,
		events:    make(chan store.QueryEvent, cfg.EventQueueCap),
		startedAt: time.Now(),
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleDNS)

	s.udp = &dns.Server{Addr: s.cfg.DNSAddr, Net: "udp", Handler: mux}
	s.tcp = &dns.Server{Addr: s.cfg.DNSAddr, Net: "tcp", Handler: mux}

	go s.eventWriter(ctx)
	go func() {
		<-ctx.Done()
		_ = s.Shutdown()
	}()
	go func() {
		s.logger.Info("dns tcp listener starting", "addr", s.cfg.DNSAddr)
		if err := s.tcp.ListenAndServe(); err != nil {
			s.logger.Warn("dns tcp listener stopped", "error", err)
		}
	}()
	s.logger.Info("dns udp listener starting", "addr", s.cfg.DNSAddr)
	return s.udp.ListenAndServe()
}

func (s *Server) Shutdown() error {
	var first error
	if s.udp != nil {
		if err := s.udp.Shutdown(); err != nil {
			first = err
		}
	}
	if s.tcp != nil {
		if err := s.tcp.Shutdown(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Server) Status() Status {
	return Status{
		StartedAt:       s.startedAt,
		UptimeSeconds:   int64(time.Since(s.startedAt).Seconds()),
		Queries:         s.queries.Load(),
		Blocked:         s.blocked.Load(),
		DroppedEvents:   s.dropped.Load(),
		DNSAddr:         s.cfg.DNSAddr,
		Upstream:        s.cfg.Upstream,
		EventQueueDepth: len(s.events),
	}
}

func (s *Server) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	start := time.Now()
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = false

	sourceIP := remoteIP(w.RemoteAddr())
	hostID, hostLabel, err := s.store.EnsureHost(context.Background(), sourceIP)
	if err != nil {
		s.logger.Warn("ensure host failed", "source_ip", sourceIP, "error", err)
	}

	action := "allowed"
	matchedSource := ""
	var matchedRuleID *int64
	upstream := ""
	answerSummary := ""
	responseCode := "NOERROR"
	qname := ""
	qtype := ""

	if len(r.Question) == 0 {
		msg.Rcode = dns.RcodeFormatError
		responseCode = dns.RcodeToString[msg.Rcode]
		_ = w.WriteMsg(msg)
		return
	}

	q := r.Question[0]
	qname = store.NormalizeName(q.Name)
	qtype = dns.TypeToString[q.Qtype]
	if qtype == "" {
		qtype = fmt.Sprintf("TYPE%d", q.Qtype)
	}

	if rule, err := s.store.MatchRule(context.Background(), qname); err == nil && rule != nil {
		ruleID := rule.ID
		matchedRuleID = &ruleID
		matchedSource = "rule:" + rule.Target
		s.store.RecordRuleHit(context.Background(), rule.ID)
		if rule.Action == "allow" {
			action = "allowed"
		} else {
			action = "blocked"
			s.blocked.Add(1)
			s.addSinkholeAnswer(msg, q)
			answerSummary = "sinkhole"
		}
	} else if err != nil {
		s.logger.Warn("rule match failed", "query", qname, "error", err)
	}

	if action != "blocked" && matchedRuleID == nil {
		if match, err := s.store.MatchBlocklist(context.Background(), qname); err == nil && match != nil {
			action = "blocked"
			matchedSource = match.SourceType + ":" + match.SourceName
			s.blocked.Add(1)
			s.addSinkholeAnswer(msg, q)
			answerSummary = "sinkhole"
		} else if err != nil {
			s.logger.Warn("blocklist match failed", "query", qname, "error", err)
		}
	}

	if action != "blocked" && len(msg.Answer) == 0 {
		if staticAnswer, ok := s.staticAnswer(context.Background(), q); ok {
			msg.Answer = append(msg.Answer, staticAnswer...)
			action = "static"
			answerSummary = summarizeAnswers(msg.Answer)
		}
	}

	if action != "blocked" && action != "static" {
		upstream = s.cfg.Upstream
		resp, err := s.forward(r)
		if err != nil {
			s.logger.Warn("upstream failed", "query", qname, "source_ip", sourceIP, "error", err)
			msg.Rcode = dns.RcodeServerFailure
			responseCode = dns.RcodeToString[msg.Rcode]
			action = "upstream_failed"
		} else {
			msg = resp
			responseCode = dns.RcodeToString[msg.Rcode]
			answerSummary = summarizeAnswers(msg.Answer)
		}
	}

	if responseCode == "NOERROR" {
		responseCode = dns.RcodeToString[msg.Rcode]
	}
	if responseCode == "" {
		responseCode = fmt.Sprintf("RCODE%d", msg.Rcode)
	}

	if err := w.WriteMsg(msg); err != nil {
		s.logger.Warn("write dns response failed", "query", qname, "source_ip", sourceIP, "error", err)
	}

	s.queries.Add(1)
	event := store.QueryEvent{
		Timestamp:     time.Now().UTC(),
		HostID:        hostID,
		SourceIP:      sourceIP,
		HostLabel:     hostLabel,
		QueryName:     qname,
		QueryType:     qtype,
		Action:        action,
		MatchedRuleID: matchedRuleID,
		MatchedSource: matchedSource,
		ResponseCode:  responseCode,
		Upstream:      upstream,
		LatencyMS:     time.Since(start).Milliseconds(),
		AnswerSummary: answerSummary,
	}
	s.logger.Debug("dns query", "source_ip", sourceIP, "host", hostLabel, "query", qname, "type", qtype, "action", action, "latency_ms", event.LatencyMS, "rcode", responseCode, "answer", answerSummary)
	select {
	case s.events <- event:
	default:
		s.dropped.Add(1)
		s.logger.Warn("event queue full; dropping query event", "query", qname, "source_ip", sourceIP)
	}
}

func (s *Server) staticAnswer(ctx context.Context, q dns.Question) ([]dns.RR, bool) {
	records, err := s.store.StaticRecords(ctx)
	if err != nil {
		s.logger.Warn("load static records failed", "error", err)
		return nil, false
	}
	qname := store.NormalizeName(q.Name)
	qtype := dns.TypeToString[q.Qtype]
	var answers []dns.RR
	for _, record := range records {
		if record.Name != qname || record.Type != qtype {
			continue
		}
		header := dns.RR_Header{Name: qname, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: uint32(record.TTL)}
		switch q.Qtype {
		case dns.TypeA:
			ip := net.ParseIP(record.Value).To4()
			if ip != nil {
				answers = append(answers, &dns.A{Hdr: header, A: ip})
			}
		case dns.TypeAAAA:
			ip := net.ParseIP(record.Value)
			if ip != nil {
				answers = append(answers, &dns.AAAA{Hdr: header, AAAA: ip})
			}
		case dns.TypeCNAME:
			answers = append(answers, &dns.CNAME{Hdr: header, Target: store.NormalizeName(record.Value)})
		case dns.TypeTXT:
			answers = append(answers, &dns.TXT{Hdr: header, Txt: []string{record.Value}})
		}
	}
	return answers, len(answers) > 0
}

func (s *Server) addSinkholeAnswer(msg *dns.Msg, q dns.Question) {
	switch q.Qtype {
	case dns.TypeA:
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.ParseIP(s.cfg.SinkholeIPv4).To4(),
		})
	case dns.TypeAAAA:
		msg.Answer = append(msg.Answer, &dns.AAAA{
			Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 30},
			AAAA: net.ParseIP(s.cfg.SinkholeIPv6),
		})
	default:
		msg.Rcode = dns.RcodeNameError
	}
}

func (s *Server) forward(r *dns.Msg) (*dns.Msg, error) {
	client := dns.Client{Net: "udp", Timeout: s.cfg.QueryTimeout}
	resp, _, err := client.Exchange(r, s.cfg.Upstream)
	if err == nil {
		return resp, nil
	}
	client.Net = "tcp"
	resp, _, tcpErr := client.Exchange(r, s.cfg.Upstream)
	if tcpErr != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Server) eventWriter(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-s.events:
			writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := s.store.InsertQueryEvent(writeCtx, event)
			cancel()
			if err != nil {
				s.logger.Warn("insert query event failed", "query", event.QueryName, "error", err)
			}
		}
	}
}

func remoteIP(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

func summarizeAnswers(answers []dns.RR) string {
	if len(answers) == 0 {
		return ""
	}
	parts := make([]string, 0, len(answers))
	for _, answer := range answers {
		switch rr := answer.(type) {
		case *dns.A:
			parts = append(parts, rr.A.String())
		case *dns.AAAA:
			parts = append(parts, rr.AAAA.String())
		case *dns.CNAME:
			parts = append(parts, rr.Target)
		default:
			fields := strings.Fields(answer.String())
			if len(fields) > 0 {
				parts = append(parts, fields[len(fields)-1])
			}
		}
		if len(parts) >= 3 {
			break
		}
	}
	return strings.Join(parts, ", ")
}
