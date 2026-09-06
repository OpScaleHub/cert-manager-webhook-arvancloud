package provider

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// stubDNS is a UDP DNS server on 127.0.0.1 that answers TXT queries with
// wantValue and records every qname it is asked for.
type stubDNS struct {
	addr string
	mu   sync.Mutex
	seen []string
}

func (s *stubDNS) queries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func startStubDNS(t *testing.T, wantValue string) *stubDNS {
	t.Helper()
	s := &stubDNS{}
	h := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		for _, q := range r.Question {
			s.mu.Lock()
			s.seen = append(s.seen, q.Name)
			s.mu.Unlock()
			if q.Qtype == dns.TypeTXT && strings.HasPrefix(q.Name, "_acme-challenge.") {
				rr, _ := dns.NewRR(q.Name + " 60 IN TXT \"" + wantValue + "\"")
				m.Answer = append(m.Answer, rr)
			}
		}
		_ = w.WriteMsg(m)
	})

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.addr = pc.LocalAddr().String()
	srv := &dns.Server{PacketConn: pc, Handler: h}
	started := make(chan struct{})
	srv.NotifyStartedFunc = func() { close(started) }
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return s
}

func TestWaitForPropagationUsesAbsoluteName(t *testing.T) {
	const val = "tok-abc"
	stub := startStubDNS(t, val)

	orig := publicResolvers
	publicResolvers = []string{stub.addr}
	t.Cleanup(func() { publicResolvers = orig })

	// Caller passes the FQDN without a trailing dot; the check must still
	// query it as an absolute name (no resolv.conf search list appended).
	if err := waitForPropagation(context.Background(), "_acme-challenge.www.example.com", val, 5); err != nil {
		t.Fatalf("waitForPropagation: %v", err)
	}

	got := stub.queries()
	if len(got) == 0 {
		t.Fatal("stub DNS received no queries")
	}
	for _, q := range got {
		if q != "_acme-challenge.www.example.com." {
			t.Fatalf("unexpected query %q — resolv.conf search list was applied", q)
		}
	}
}

func TestWaitForPropagationTimesOut(t *testing.T) {
	stub := startStubDNS(t, "different-value")
	orig := publicResolvers
	publicResolvers = []string{stub.addr}
	t.Cleanup(func() { publicResolvers = orig })

	start := time.Now()
	err := waitForPropagation(context.Background(), "_acme-challenge.x.example.com.", "expected", 2)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}
