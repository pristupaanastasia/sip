package transport

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/sip"
)

func TestEndpointSend(t *testing.T) {
	conn := listenLoopback(t)
	receiver := listenLoopback(t)
	var logs bytes.Buffer
	endpoint := Endpoint{Conn: conn, Log: logging.NewWithWriter("test", &logs)}
	msg := &sip.Message{StatusCode: 200, Reason: "OK", Headers: []sip.Header{{Name: "Call-ID", Value: "abc"}}}

	endpoint.Send(msg, receiver.LocalAddr().(*net.UDPAddr))

	if deadlineErr := receiver.SetReadDeadline(time.Now().Add(time.Second)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}
	buf := make([]byte, MaxDatagram)
	n, _, readErr := receiver.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if got := string(buf[:n]); got != string(msg.Bytes()) {
		t.Errorf("received %q, want %q", got, msg.Bytes())
	}
	if !strings.Contains(logs.String(), `msg="sent message"`) {
		t.Errorf("log does not contain the send message:\n%s", logs.String())
	}
}

func TestEndpointSendLogsFailure(t *testing.T) {
	conn := listenLoopback(t)
	var logs bytes.Buffer
	endpoint := Endpoint{Conn: conn, Log: logging.NewWithWriter("test", &logs)}
	conn.Close()

	endpoint.Send(&sip.Message{StatusCode: 200, Reason: "OK"}, conn.LocalAddr().(*net.UDPAddr))

	if !strings.Contains(logs.String(), `msg="send failed"`) {
		t.Errorf("log does not contain the failure:\n%s", logs.String())
	}
}

func TestEndpointReceive(t *testing.T) {
	const validRequest = "OPTIONS sip:bob@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP c;branch=z9hG4bK1\r\n" +
		"From: <sip:a@x>;tag=1\r\nTo: <sip:b@x>\r\nCall-ID: abc\r\nCSeq: 1 OPTIONS\r\n\r\n"

	tests := map[string]struct {
		data      string
		wantOK    bool
		wantReply int
	}{
		"valid request": {
			data:   validRequest,
			wantOK: true,
		},
		"garbage is dropped silently": {
			data: "hello",
		},
		"invalid request gets 400": {
			data:      "OPTIONS sip:bob@example.com SIP/2.0\r\nVia: SIP/2.0/UDP c;branch=z9hG4bK1\r\nCSeq: 1 OPTIONS\r\n\r\n",
			wantReply: 400,
		},
		"short body gets 400": {
			data:      strings.Replace(validRequest, "\r\n\r\n", "\r\nContent-Length: 50\r\n\r\nshort", 1),
			wantReply: 400,
		},
		"invalid ACK gets no reply": {
			data: "ACK sip:bob@example.com SIP/2.0\r\nCSeq: 1 ACK\r\n\r\n",
		},
		"invalid response gets no reply": {
			data: "SIP/2.0 200 OK\r\nCall-ID: abc\r\n\r\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			conn := listenLoopback(t)
			peer := listenLoopback(t)
			endpoint := Endpoint{Conn: conn, Log: logging.NewWithWriter("test", &bytes.Buffer{})}

			msg, ok := endpoint.Receive([]byte(tc.data), peer.LocalAddr().(*net.UDPAddr))

			if ok != tc.wantOK || (msg != nil) != tc.wantOK {
				t.Fatalf("Receive() = %v, %v, want ok = %v", msg, ok, tc.wantOK)
			}
			if got := replyCode(t, peer); got != tc.wantReply {
				t.Errorf("reply code = %d, want %d (0 means no reply)", got, tc.wantReply)
			}
		})
	}
}

func replyCode(t *testing.T, conn *net.UDPConn) int {
	t.Helper()
	if deadlineErr := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}
	buf := make([]byte, MaxDatagram)
	n, _, readErr := conn.ReadFromUDP(buf)
	if readErr != nil {
		return 0
	}
	reply, parseErr := sip.Parse(buf[:n])
	if parseErr != nil {
		t.Fatalf("reply is not a SIP message: %v", parseErr)
	}
	return reply.StatusCode
}
