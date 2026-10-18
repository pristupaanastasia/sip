package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pristupaanastasia/sip/internal/sip"
)

func TestFinalResponse(t *testing.T) {
	requestHeaders := []sip.Header{
		{Name: "Via", Value: "SIP/2.0/UDP proxy;branch=z9hG4bK2"},
		{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
		{Name: "Max-Forwards", Value: "69"},
		{Name: "From", Value: "<sip:alice@example.com>;tag=1"},
		{Name: "Call-ID", Value: "abc"},
		{Name: "CSeq", Value: "1 INVITE"},
	}
	template := &sip.Message{
		StatusCode: 200,
		Reason:     "OK",
		Headers: []sip.Header{
			{Name: "Via", Value: "SIP/2.0/UDP template;branch=z9hG4bKtemplate"},
			{Name: "To", Value: "<sip:bob@example.com>;tag=template"},
			{Name: "Call-ID", Value: "template-call"},
			{Name: "CSeq", Value: "99 INVITE"},
			{Name: "Contact", Value: "<sip:bob@server>"},
			{Name: "Content-Type", Value: "application/sdp"},
			{Name: "Content-Length", Value: "5"},
		},
		Body: []byte("v=0\r\n"),
	}

	tests := map[string]struct {
		to     string
		wantTo string
	}{
		"keeps existing To tag": {
			to:     "<sip:bob@example.com>;tag=existing",
			wantTo: "<sip:bob@example.com>;tag=existing",
		},
		"adds To tag when missing": {
			to:     "<sip:bob@example.com>",
			wantTo: "<sip:bob@example.com>;tag=",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			headers := append([]sip.Header{}, requestHeaders...)
			headers = append(headers, sip.Header{Name: "To", Value: tc.to})
			req := &sip.Message{IsRequest: true, Method: "INVITE", RequestURI: "sip:bob@example.com", Headers: headers}
			a := &app{template: template}

			got, gotErr := a.finalResponse(req)
			if gotErr != nil {
				t.Fatalf("finalResponse() error = %v", gotErr)
			}

			if got.StatusCode != 200 || got.Reason != "OK" {
				t.Errorf("status = %d %s, want 200 OK", got.StatusCode, got.Reason)
			}
			if !reflect.DeepEqual(got.Values("Via"), []string{"SIP/2.0/UDP proxy;branch=z9hG4bK2", "SIP/2.0/UDP client;branch=z9hG4bK1"}) {
				t.Errorf("Via = %q, want request Via in order", got.Values("Via"))
			}
			for name, want := range map[string]string{
				"From":         "<sip:alice@example.com>;tag=1",
				"Call-ID":      "abc",
				"CSeq":         "1 INVITE",
				"Contact":      "<sip:bob@server>",
				"Content-Type": "application/sdp",
			} {
				if value, _ := got.Get(name); value != want {
					t.Errorf("%s = %q, want %q", name, value, want)
				}
			}
			if to, _ := got.Get("To"); !strings.HasPrefix(to, tc.wantTo) || strings.Count(to, ";tag=") != 1 {
				t.Errorf("To = %q, want prefix %q with exactly one tag", to, tc.wantTo)
			}
			if _, ok := got.Get("Max-Forwards"); ok {
				t.Error("response must not contain Max-Forwards")
			}
			if string(got.Body) != "v=0\r\n" {
				t.Errorf("Body = %q, want template body", got.Body)
			}
		})
	}
}

func TestLoadResponse(t *testing.T) {
	tests := map[string]struct {
		content string
		missing bool
		wantErr error
	}{
		"valid response": {
			content: "SIP/2.0 200 OK\r\nContent-Length: 0\r\n\r\n",
		},
		"file contains a request": {
			content: "INVITE sip:bob@example.com SIP/2.0\r\n\r\n",
			wantErr: errNotResponse,
		},
		"malformed file": {
			content: "SIP/2.0 abc OK\r\n\r\n",
			wantErr: sip.ErrInvalidStatusCode,
		},
		"missing file": {
			missing: true,
			wantErr: os.ErrNotExist,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "response.txt")
			if !tc.missing {
				if writeErr := os.WriteFile(path, []byte(tc.content), 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
			}

			got, gotErr := loadResponse(path)

			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("loadResponse() error = %v, want %v", gotErr, tc.wantErr)
			}
			if tc.wantErr == nil && (got == nil || got.IsRequest) {
				t.Errorf("loadResponse() = %+v, want a response", got)
			}
		})
	}
}

func TestToTagIsStablePerDialog(t *testing.T) {
	a := &app{tagSecret: []byte("secret")}
	dialog := func(callID, fromTag, cseq string) *sip.Message {
		return &sip.Message{IsRequest: true, Method: "INVITE", Headers: []sip.Header{
			{Name: "From", Value: "<sip:alice@example.com>;tag=" + fromTag},
			{Name: "Call-ID", Value: callID},
			{Name: "CSeq", Value: cseq},
		}}
	}
	first, firstErr := a.toTag(dialog("abc", "1", "1 INVITE"))
	if firstErr != nil {
		t.Fatalf("toTag() error = %v", firstErr)
	}

	tests := map[string]struct {
		req      *sip.Message
		wantSame bool
	}{
		"retransmission":         {req: dialog("abc", "1", "1 INVITE"), wantSame: true},
		"next request in dialog": {req: dialog("abc", "1", "2 INVITE"), wantSame: true},
		"other Call-ID":          {req: dialog("other", "1", "1 INVITE"), wantSame: false},
		"other From tag":         {req: dialog("abc", "2", "1 INVITE"), wantSame: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got, _ := a.toTag(tc.req); (got == first) != tc.wantSame {
				t.Errorf("toTag() = %q, first = %q, want same = %v", got, first, tc.wantSame)
			}
		})
	}

	if other, _ := (&app{tagSecret: []byte("other secret")}).toTag(dialog("abc", "1", "1 INVITE")); other == first {
		t.Error("toTag() does not depend on the secret")
	}
	if len(first) != 16 {
		t.Errorf("toTag() = %q, want 16 hex characters (64 bits)", first)
	}
}

func TestToTagMissingHeader(t *testing.T) {
	tests := map[string]struct {
		headers []sip.Header
	}{
		"without Call-ID": {headers: []sip.Header{{Name: "From", Value: "<sip:alice@example.com>;tag=1"}}},
		"without From":    {headers: []sip.Header{{Name: "Call-ID", Value: "abc"}}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := &app{tagSecret: []byte("secret")}
			req := &sip.Message{IsRequest: true, Method: "INVITE", Headers: tc.headers}

			got, gotErr := a.toTag(req)

			if !errors.Is(gotErr, errMissingDialogHeader) {
				t.Fatalf("toTag() error = %v, want %v", gotErr, errMissingDialogHeader)
			}
			if got != "" {
				t.Errorf("toTag() = %q, want empty on error", got)
			}
		})
	}
}
