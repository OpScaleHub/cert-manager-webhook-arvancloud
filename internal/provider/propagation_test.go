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

// startStubDNS runs a UDP DNS server on 127.0.0.1 that answers TXT queries
// with wantValue, and records every qname it was asked for.
func startStubDNS(t *testing.T, wantValue string) (addr string, queries *[]string) {
	t.Helper()
	var (
		mu   sync.Mutex
		seen []string
	)
	h := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		for _, q := range r.Question {
			mu.Lock()
			seen = append(seen, q.Name)
			mu.Unlock()
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
	srv := &dns.Server{PacketConn: pc, Handler: h}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	return pc.LocalAddr().String(), &seen
}

func TestWaitForPropagationUsesAbsoluteName(t *testing.T) {
	const val = "tok-abc"
	addr, seen := startStubDNS(t, val)

	orig := publicResolvers
	publicResolvers = []string{addr}
	t.Cleanup(func() { publicResolvers = orig })

	// Caller passes the FQDN without a trailing dot; the check must still
	// query it as an absolute name (no resolv.conf search list appended).
	err := waitForPropagation(context.Background(), "_acme-challenge.www.example.com", val, 5)
	if err != nil {
		t.Fatalf("waitForPropagation: %v", err)
	}

	for _, q := range *seen {
		if q != "_acme-challenge.www.example.com." {
			t.Fatalf("unexpected query %q — search list was applied", q)
		}
	}
	if len(*seen) == 0 {
		t.Fatal("stub DNS received no queries")
	}
}

func TestWaitForPropagationTimesOut(t *testing.T) {
	addr, _ := startStubDNS(t, "different-value")
	orig := publicResolvers
	publicResolvers = []string{addr}
	t.Cleanup(func() { publicResolvers = orig })

	start := time.Now()
	err := waitForPropagation(context.Background(), "_acme-challenge.x.example.com.", "expected", 2)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}
