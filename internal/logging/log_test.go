package logging

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/pristupaanastasia/sip/internal/sip"
)

func TestFormatReceived(t *testing.T) {
	tests := map[string]struct {
		msg  *sip.Message
		want string
	}{
		"request shows Request-URI": {
			msg: &sip.Message{
				IsRequest:  true,
				Method:     "INVITE",
				RequestURI: "sip:bob@example.com",
				Headers: []sip.Header{
					{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
					{Name: "Call-ID", Value: "abc"},
				},
			},
			want: "Received request:\n" +
				"sip:bob@example.com\n" +
				"Headers: {'Via': 'SIP/2.0/UDP client;branch=z9hG4bK1', 'Call-ID': 'abc'}\n",
		},
		"response shows Status-Line": {
			msg: &sip.Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []sip.Header{{Name: "CSeq", Value: "1 INVITE"}},
			},
			want: "Received response:\n" +
				"SIP/2.0 200 OK\n" +
				"Headers: {'CSeq': '1 INVITE'}\n",
		},
		"repeated headers keep their order": {
			msg: &sip.Message{
				StatusCode: 100,
				Reason:     "Trying",
				Headers: []sip.Header{
					{Name: "Via", Value: "first"},
					{Name: "Via", Value: "second"},
				},
			},
			want: "Received response:\n" +
				"SIP/2.0 100 Trying\n" +
				"Headers: {'Via': 'first', 'Via': 'second'}\n",
		},
		"quotes and backslashes are escaped": {
			msg: &sip.Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []sip.Header{{Name: "Subject", Value: `it's a \ test`}},
			},
			want: "Received response:\n" +
				"SIP/2.0 200 OK\n" +
				`Headers: {'Subject': 'it\'s a \\ test'}` + "\n",
		},
		"no headers": {
			msg:  &sip.Message{StatusCode: 200, Reason: "OK"},
			want: "Received response:\nSIP/2.0 200 OK\nHeaders: {}\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := FormatReceived(tc.msg); got != tc.want {
				t.Errorf("FormatReceived() =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestLoggerReceived(t *testing.T) {
	var out bytes.Buffer
	lg := NewWithWriter("proxy", &out)
	msg := &sip.Message{StatusCode: 200, Reason: "OK", Headers: []sip.Header{{Name: "Call-ID", Value: "abc"}}}
	from := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5070}

	lg.Received(from, msg)

	got := out.String()
	wantParts := []string{
		"component=proxy",
		"from=127.0.0.1:5070",
		FormatReceived(msg),
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Errorf("output does not contain %q:\n%s", part, got)
		}
	}
}
