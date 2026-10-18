package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/proxy"
	"github.com/pristupaanastasia/sip/internal/sip"
	"github.com/pristupaanastasia/sip/internal/transport"
)

func TestRejectResponse(t *testing.T) {
	req := &sip.Message{IsRequest: true, Method: "INVITE", Headers: []sip.Header{{Name: "Call-ID", Value: "abc"}}}

	tests := map[string]struct {
		err        error
		wantCode   int
		wantReason string
	}{
		"too many hops":         {err: proxy.ErrTooManyHops, wantCode: 483, wantReason: "Too Many Hops"},
		"wrapped too many hops": {err: fmt.Errorf("x: %w", proxy.ErrTooManyHops), wantCode: 483, wantReason: "Too Many Hops"},
		"invalid Max-Forwards":  {err: proxy.ErrInvalidMaxForwards, wantCode: 400, wantReason: "Bad Request"},
		"any other error":       {err: errors.New("boom"), wantCode: 400, wantReason: "Bad Request"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := rejectResponse(req, tc.err)
			if got.StatusCode != tc.wantCode || got.Reason != tc.wantReason {
				t.Errorf("rejectResponse() = %d %s, want %d %s", got.StatusCode, got.Reason, tc.wantCode, tc.wantReason)
			}
			if callID, _ := got.Get("Call-ID"); callID != "abc" {
				t.Errorf("Call-ID = %q, want copied from request", callID)
			}
		})
	}
}

func TestProxyForwardsRequestAndResponse(t *testing.T) {
	proxyConn, client, upstream := listenThree(t)
	a := &app{
		endpoint: transport.Endpoint{Conn: proxyConn, Log: logging.NewWithWriter("proxy", io.Discard)},
		upstream: upstream.LocalAddr().(*net.UDPAddr),
		viaHost:  "proxy.test:5060",
		pending:  proxy.NewPending(time.Minute),
		log:      logging.NewWithWriter("proxy", io.Discard),
	}
	clientAddr := client.LocalAddr().(*net.UDPAddr)

	invite := "INVITE sip:bob@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP client;branch=z9hG4bK1\r\n" +
		"Max-Forwards: 70\r\n" +
		"From: <sip:alice@example.com>;tag=1\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"Call-ID: abc\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"\r\n"
	a.handle([]byte(invite), clientAddr)

	trying := readMessage(t, client)
	if trying.StatusCode != 100 {
		t.Fatalf("client got %q, want 100 Trying", trying.StartLine())
	}

	forwarded := readMessage(t, upstream)
	if maxForwards, _ := forwarded.Get("Max-Forwards"); maxForwards != "69" {
		t.Errorf("forwarded Max-Forwards = %q, want 69", maxForwards)
	}
	vias := forwarded.Values("Via")
	if len(vias) != 2 || vias[1] != "SIP/2.0/UDP client;branch=z9hG4bK1" {
		t.Fatalf("forwarded Via = %q, want proxy Via on top of client Via", vias)
	}
	branch := sip.ViaBranch(vias[0])

	upstreamAddr := upstream.LocalAddr().(*net.UDPAddr)
	a.handle(sip.NewResponse(forwarded, 100, "Trying").Bytes(), upstreamAddr)
	a.handle(sip.NewResponse(forwarded, 200, "OK").Bytes(), upstreamAddr)

	final := readMessage(t, client)
	if final.StatusCode != 200 {
		t.Fatalf("client got %q, want 200 OK (upstream 100 Trying must not be forwarded)", final.StartLine())
	}
	if got := final.Values("Via"); len(got) != 1 || got[0] != "SIP/2.0/UDP client;branch=z9hG4bK1" {
		t.Errorf("final Via = %q, want only the client Via", got)
	}
	if _, stillPending := a.pending.Get(branch); stillPending {
		t.Error("transaction is still pending after the final response")
	}
}

func TestProxyAbsorbsRetransmission(t *testing.T) {
	proxyConn, client, upstream := listenThree(t)
	a := &app{
		endpoint:     transport.Endpoint{Conn: proxyConn, Log: logging.NewWithWriter("proxy", io.Discard)},
		upstream:     upstream.LocalAddr().(*net.UDPAddr),
		viaHost:      "proxy.test:5060",
		branchSecret: []byte("secret"),
		pending:      proxy.NewPending(time.Minute),
		log:          logging.NewWithWriter("proxy", io.Discard),
	}
	invite := "INVITE sip:bob@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP client;branch=z9hG4bK1\r\n" +
		"From: <sip:alice@example.com>;tag=1\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"Call-ID: abc\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"\r\n"

	a.handle([]byte(invite), client.LocalAddr().(*net.UDPAddr))
	a.handle([]byte(invite), client.LocalAddr().(*net.UDPAddr))

	for range 2 {
		if got := readMessage(t, client); got.StatusCode != 100 {
			t.Fatalf("client got %q, want 100 Trying for every copy", got.StartLine())
		}
	}
	readMessage(t, upstream)
	if upstream.SetReadDeadline(time.Now().Add(50*time.Millisecond)) == nil {
		if _, _, readErr := upstream.ReadFromUDP(make([]byte, 1)); readErr == nil {
			t.Error("retransmission was forwarded upstream")
		}
	}
}

func TestProxyRejectsRequest(t *testing.T) {
	tests := map[string]struct {
		request  string
		wantCode int
	}{
		"Max-Forwards zero": {
			request: "OPTIONS sip:bob@example.com SIP/2.0\r\nVia: SIP/2.0/UDP c;branch=z9hG4bK1\r\nMax-Forwards: 0\r\n" +
				"From: <sip:a@x>;tag=1\r\nTo: <sip:b@x>\r\nCall-ID: abc\r\nCSeq: 1 OPTIONS\r\n\r\n",
			wantCode: 483,
		},
		"missing Call-ID": {
			request: "OPTIONS sip:bob@example.com SIP/2.0\r\nVia: SIP/2.0/UDP c;branch=z9hG4bK1\r\n" +
				"From: <sip:a@x>;tag=1\r\nTo: <sip:b@x>\r\nCSeq: 1 OPTIONS\r\n\r\n",
			wantCode: 400,
		},
		"body shorter than Content-Length": {
			request: "OPTIONS sip:bob@example.com SIP/2.0\r\nVia: SIP/2.0/UDP c;branch=z9hG4bK1\r\n" +
				"From: <sip:a@x>;tag=1\r\nTo: <sip:b@x>\r\nCall-ID: abc\r\nCSeq: 1 OPTIONS\r\nContent-Length: 50\r\n\r\nshort",
			wantCode: 400,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			proxyConn, client, upstream := listenThree(t)
			a := &app{
				endpoint: transport.Endpoint{Conn: proxyConn, Log: logging.NewWithWriter("proxy", io.Discard)},
				upstream: upstream.LocalAddr().(*net.UDPAddr),
				viaHost:  "proxy.test:5060",
				pending:  proxy.NewPending(time.Minute),
				log:      logging.NewWithWriter("proxy", io.Discard),
			}

			a.handle([]byte(tc.request), client.LocalAddr().(*net.UDPAddr))

			if got := readMessage(t, client); got.StatusCode != tc.wantCode {
				t.Errorf("client got %q, want %d", got.StartLine(), tc.wantCode)
			}
			if upstream.SetReadDeadline(time.Now().Add(50*time.Millisecond)) == nil {
				if _, _, readErr := upstream.ReadFromUDP(make([]byte, 1)); readErr == nil {
					t.Error("rejected request was forwarded upstream")
				}
			}
		})
	}
}

func listenThree(t *testing.T) (proxyConn, client, upstream *net.UDPConn) {
	t.Helper()
	conns := make([]*net.UDPConn, 3)
	for i := range conns {
		conn, listenErr := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		t.Cleanup(func() { conn.Close() })
		conns[i] = conn
	}
	return conns[0], conns[1], conns[2]
}

func readMessage(t *testing.T, conn *net.UDPConn) *sip.Message {
	t.Helper()
	if deadlineErr := conn.SetReadDeadline(time.Now().Add(time.Second)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}
	buf := make([]byte, 65535)
	n, _, readErr := conn.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	msg, parseErr := sip.Parse(buf[:n])
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	return msg
}
