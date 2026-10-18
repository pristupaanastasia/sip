package proxy

import (
	"errors"
	"strings"
	"testing"

	"github.com/pristupaanastasia/sip/internal/sip"
)

func TestBranch(t *testing.T) {
	secret := []byte("secret")
	request := func(topVia, callID, cseq, uri string) *sip.Message {
		return &sip.Message{IsRequest: true, Method: "INVITE", RequestURI: uri, Headers: []sip.Header{
			{Name: "Via", Value: topVia},
			{Name: "Via", Value: "SIP/2.0/UDP below;branch=z9hG4bKbelow"},
			{Name: "Call-ID", Value: callID},
			{Name: "CSeq", Value: cseq},
		}}
	}
	base := request("SIP/2.0/UDP client;branch=z9hG4bK1", "abc", "1 INVITE", "sip:bob@example.com")
	want, wantErr := Branch(secret, base)
	if wantErr != nil {
		t.Fatalf("Branch(base) error = %v", wantErr)
	}

	tests := map[string]struct {
		req      *sip.Message
		wantSame bool
	}{
		"retransmission": {
			req:      request("SIP/2.0/UDP client;branch=z9hG4bK1", "abc", "1 INVITE", "sip:bob@example.com"),
			wantSame: true,
		},
		"top Via combined with others by comma": {
			req:      request("SIP/2.0/UDP client;branch=z9hG4bK1, SIP/2.0/UDP other", "abc", "1 INVITE", "sip:bob@example.com"),
			wantSame: true,
		},
		"other client branch": {
			req: request("SIP/2.0/UDP client;branch=z9hG4bK2", "abc", "1 INVITE", "sip:bob@example.com"),
		},
		"other Call-ID": {
			req: request("SIP/2.0/UDP client;branch=z9hG4bK1", "other", "1 INVITE", "sip:bob@example.com"),
		},
		"other CSeq": {
			req: request("SIP/2.0/UDP client;branch=z9hG4bK1", "abc", "2 INVITE", "sip:bob@example.com"),
		},
		"other Request-URI": {
			req: request("SIP/2.0/UDP client;branch=z9hG4bK1", "abc", "1 INVITE", "sip:carol@example.com"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := Branch(secret, tc.req)
			if gotErr != nil {
				t.Fatalf("Branch() error = %v", gotErr)
			}
			if (got == want) != tc.wantSame {
				t.Errorf("Branch() = %q, base = %q, want same = %v", got, want, tc.wantSame)
			}
		})
	}

	if !strings.HasPrefix(want, magicCookie) || len(want) != len(magicCookie)+16 {
		t.Errorf("Branch() = %q, want %q followed by 16 hex characters", want, magicCookie)
	}
	if other, _ := Branch([]byte("other secret"), base); other == want {
		t.Error("Branch() does not depend on the secret")
	}
}

func TestBranchMissingHeader(t *testing.T) {
	headers := []sip.Header{
		{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
		{Name: "Call-ID", Value: "abc"},
		{Name: "CSeq", Value: "1 INVITE"},
	}

	tests := map[string]struct {
		missing string
	}{
		"without Via":     {missing: "Via"},
		"without Call-ID": {missing: "Call-ID"},
		"without CSeq":    {missing: "CSeq"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kept := make([]sip.Header, 0, len(headers))
			for _, h := range headers {
				if h.Name != tc.missing {
					kept = append(kept, h)
				}
			}
			req := &sip.Message{IsRequest: true, Method: "INVITE", RequestURI: "sip:bob@example.com", Headers: kept}

			got, gotErr := Branch([]byte("secret"), req)

			if !errors.Is(gotErr, ErrMissingTransactionHeader) {
				t.Fatalf("Branch() error = %v, want %v", gotErr, ErrMissingTransactionHeader)
			}
			if got != "" {
				t.Errorf("Branch() = %q, want empty on error", got)
			}
		})
	}
}

func TestNewSecret(t *testing.T) {
	first, second := NewSecret(), NewSecret()
	if len(first) != 32 {
		t.Errorf("len(NewSecret()) = %d, want 32", len(first))
	}
	if string(first) == string(second) {
		t.Error("NewSecret() returned the same secret twice")
	}
}
