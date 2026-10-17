package sip

import (
	"os"
	"reflect"
	"testing"
)

func TestMessageBytes(t *testing.T) {
	tests := map[string]struct {
		msg  *Message
		want string
	}{
		"request with body": {
			msg: &Message{
				IsRequest:  true,
				Method:     "INVITE",
				RequestURI: "sip:bob@example.com",
				Headers: []Header{
					{Name: "Call-ID", Value: "abc"},
					{Name: "Content-Length", Value: "999"},
					{Name: "Content-Type", Value: "application/sdp"},
				},
				Body: []byte("v=0\r\n"),
			},
			want: "INVITE sip:bob@example.com SIP/2.0\r\n" +
				"Call-ID: abc\r\n" +
				"Content-Length: 5\r\n" +
				"Content-Type: application/sdp\r\n" +
				"\r\n" +
				"v=0\r\n",
		},
		"response without Content-Length gets one appended": {
			msg: &Message{
				StatusCode: 100,
				Reason:     "Trying",
				Headers:    []Header{{Name: "Call-ID", Value: "abc"}},
			},
			want: "SIP/2.0 100 Trying\r\n" +
				"Call-ID: abc\r\n" +
				"Content-Length: 0\r\n" +
				"\r\n",
		},
		"duplicate Content-Length is written once": {
			msg: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers: []Header{
					{Name: "Content-Length", Value: "1"},
					{Name: "Content-Length", Value: "2"},
				},
			},
			want: "SIP/2.0 200 OK\r\n" +
				"Content-Length: 0\r\n" +
				"\r\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := string(tc.msg.Bytes()); got != tc.want {
				t.Errorf("Bytes() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestParseBytesRoundTrip(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"testdata request": {
			input: readTestdata(t, "invite.txt"),
		},
		"testdata response": {
			input: readTestdata(t, "200ok.txt"),
		},
		"LF and compact forms": {
			input: "OPTIONS sip:bob@example.com SIP/2.0\n" +
				"v: SIP/2.0/UDP a;branch=z9hG4bK1\n" +
				"i: abc\n" +
				"l: 3\n" +
				"\n" +
				"abc",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			first, firstErr := Parse([]byte(tc.input))
			if firstErr != nil {
				t.Fatalf("Parse(input) error = %v", firstErr)
			}
			second, secondErr := Parse(first.Bytes())
			if secondErr != nil {
				t.Fatalf("Parse(Bytes()) error = %v", secondErr)
			}
			if !reflect.DeepEqual(first, second) {
				t.Errorf("round trip changed message:\nfirst  %+v\nsecond %+v", first, second)
			}
		})
	}
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, readErr := os.ReadFile("../../testdata/" + name)
	if readErr != nil {
		t.Fatalf("read testdata %s: %v", name, readErr)
	}
	return string(data)
}
