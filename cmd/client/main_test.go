package main

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/sip"
)

func TestLoadRequest(t *testing.T) {
	tests := map[string]struct {
		content string
		missing bool
		wantErr error
	}{
		"valid request": {
			content: "INVITE sip:bob@example.com SIP/2.0\r\nCall-ID: abc\r\n\r\n",
		},
		"file contains a response": {
			content: "SIP/2.0 200 OK\r\n\r\n",
			wantErr: errNotRequest,
		},
		"malformed file": {
			content: "INVITE sip:bob@example.com\r\n\r\n",
			wantErr: sip.ErrMalformedStartLine,
		},
		"missing file": {
			missing: true,
			wantErr: os.ErrNotExist,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "request.txt")
			if !tc.missing {
				if writeErr := os.WriteFile(path, []byte(tc.content), 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
			}

			got, gotErr := loadRequest(path)

			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("loadRequest() error = %v, want %v", gotErr, tc.wantErr)
			}
			if tc.wantErr == nil && (got == nil || !got.IsRequest) {
				t.Errorf("loadRequest() = %+v, want a request", got)
			}
		})
	}
}

func TestSendRequest(t *testing.T) {
	const callID = "abc"
	req := testRequest("INVITE", callID)
	responseTo := func(code int, reason, responseCallID string) []byte {
		resp := sip.NewResponse(req, code, reason)
		resp.Set("Call-ID", responseCallID)
		return resp.Bytes()
	}

	tests := map[string]struct {
		datagrams [][]byte
		wantCode  int
		wantErr   error
	}{
		"provisional then final": {
			datagrams: [][]byte{responseTo(100, "Trying", callID), responseTo(200, "OK", callID)},
			wantCode:  200,
		},
		"failure final response is returned": {
			datagrams: [][]byte{responseTo(486, "Busy Here", callID)},
			wantCode:  486,
		},
		"skips garbage, requests, invalid and foreign responses": {
			datagrams: [][]byte{
				[]byte("garbage"),
				req.Bytes(),
				[]byte("SIP/2.0 200 OK\r\nCall-ID: abc\r\n\r\n"),
				responseTo(200, "OK", "other-call"),
				responseTo(200, "OK", callID),
			},
			wantCode: 200,
		},
		"only provisional responses time out": {
			datagrams: [][]byte{responseTo(100, "Trying", callID)},
			wantErr:   errNoFinalResponse,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, server := udpPair(t)
			for _, datagram := range tc.datagrams {
				if _, writeErr := server.WriteToUDP(datagram, client.LocalAddr().(*net.UDPAddr)); writeErr != nil {
					t.Fatal(writeErr)
				}
			}

			got, gotErr := sendRequest(client, req, server.LocalAddr().(*net.UDPAddr), 300*time.Millisecond, discardLogger())

			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("sendRequest() error = %v, want %v", gotErr, tc.wantErr)
			}
			if tc.wantErr == nil && got.StatusCode != tc.wantCode {
				t.Errorf("status = %d, want %d", got.StatusCode, tc.wantCode)
			}
		})
	}
}

func TestSendRequestWithoutCallID(t *testing.T) {
	client, server := udpPair(t)
	req := testRequest("INVITE", "abc")
	req.Del("Call-ID")

	_, gotErr := sendRequest(client, req, server.LocalAddr().(*net.UDPAddr), time.Second, discardLogger())

	if !errors.Is(gotErr, errMissingCallID) {
		t.Fatalf("sendRequest() error = %v, want %v", gotErr, errMissingCallID)
	}
	if got := countDatagrams(t, server); got != 0 {
		t.Errorf("server received %d datagrams, want none", got)
	}
}

func TestSendRequestRetransmits(t *testing.T) {
	tests := map[string]struct {
		answerTrying bool
		timeout      time.Duration
		wantCopies   int
	}{
		"retransmits while there is no response": {
			timeout:    1600 * time.Millisecond,
			wantCopies: 3,
		},
		"stops retransmitting after a provisional response": {
			answerTrying: true,
			timeout:      1200 * time.Millisecond,
			wantCopies:   1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, server := udpPair(t)
			req := testRequest("INVITE", "abc")
			if tc.answerTrying {
				trying := sip.NewTrying(req).Bytes()
				if _, writeErr := server.WriteToUDP(trying, client.LocalAddr().(*net.UDPAddr)); writeErr != nil {
					t.Fatal(writeErr)
				}
			}

			_, gotErr := sendRequest(client, req, server.LocalAddr().(*net.UDPAddr), tc.timeout, discardLogger())
			if !errors.Is(gotErr, errNoFinalResponse) {
				t.Fatalf("sendRequest() error = %v, want %v", gotErr, errNoFinalResponse)
			}

			if got := countDatagrams(t, server); got != tc.wantCopies {
				t.Errorf("server received %d copies, want %d", got, tc.wantCopies)
			}
		})
	}
}

func TestNextRetransmitInterval(t *testing.T) {
	tests := map[string]struct {
		method   string
		interval time.Duration
		want     time.Duration
	}{
		"INVITE doubles":             {method: "INVITE", interval: 500 * time.Millisecond, want: time.Second},
		"INVITE is not capped":       {method: "INVITE", interval: 4 * time.Second, want: 8 * time.Second},
		"non-INVITE doubles":         {method: "OPTIONS", interval: time.Second, want: 2 * time.Second},
		"non-INVITE is capped at T2": {method: "OPTIONS", interval: 4 * time.Second, want: retransmitT2},
		"non-INVITE stays at T2":     {method: "BYE", interval: retransmitT2, want: retransmitT2},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := nextRetransmitInterval(tc.method, tc.interval); got != tc.want {
				t.Errorf("nextRetransmitInterval(%s, %s) = %s, want %s", tc.method, tc.interval, got, tc.want)
			}
		})
	}
}

func testRequest(method, callID string) *sip.Message {
	return &sip.Message{IsRequest: true, Method: method, RequestURI: "sip:bob@example.com", Headers: []sip.Header{
		{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
		{Name: "From", Value: "<sip:alice@example.com>;tag=1"},
		{Name: "To", Value: "<sip:bob@example.com>"},
		{Name: "Call-ID", Value: callID},
		{Name: "CSeq", Value: "1 " + method},
	}}
}

func countDatagrams(t *testing.T, conn *net.UDPConn) int {
	t.Helper()
	buf := make([]byte, 65535)
	count := 0
	for {
		if deadlineErr := conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); deadlineErr != nil {
			t.Fatal(deadlineErr)
		}
		if _, _, readErr := conn.ReadFromUDP(buf); readErr != nil {
			return count
		}
		count++
	}
}

func discardLogger() *logging.Logger {
	return logging.NewWithWriter("client", io.Discard)
}

func udpPair(t *testing.T) (client, server *net.UDPConn) {
	t.Helper()
	loopback := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	client, clientErr := net.ListenUDP("udp", loopback)
	if clientErr != nil {
		t.Fatal(clientErr)
	}
	server, serverErr := net.ListenUDP("udp", loopback)
	if serverErr != nil {
		t.Fatal(serverErr)
	}
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return client, server
}
