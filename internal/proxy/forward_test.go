package proxy

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pristupaanastasia/sip/internal/sip"
)

func TestDecrementMaxForwards(t *testing.T) {
	tests := map[string]struct {
		headers     []sip.Header
		wantHeaders []sip.Header
		wantErr     error
	}{
		"decrements value": {
			headers:     []sip.Header{{Name: "Max-Forwards", Value: "70"}},
			wantHeaders: []sip.Header{{Name: "Max-Forwards", Value: "69"}},
		},
		"one becomes zero": {
			headers:     []sip.Header{{Name: "Max-Forwards", Value: "1"}},
			wantHeaders: []sip.Header{{Name: "Max-Forwards", Value: "0"}},
		},
		"keeps header position": {
			headers:     []sip.Header{{Name: "Via", Value: "v"}, {Name: "Max-Forwards", Value: "10"}, {Name: "Call-ID", Value: "abc"}},
			wantHeaders: []sip.Header{{Name: "Via", Value: "v"}, {Name: "Max-Forwards", Value: "9"}, {Name: "Call-ID", Value: "abc"}},
		},
		"missing header is set to default": {
			headers:     []sip.Header{{Name: "Call-ID", Value: "abc"}},
			wantHeaders: []sip.Header{{Name: "Call-ID", Value: "abc"}, {Name: "Max-Forwards", Value: "70"}},
		},
		"zero is rejected": {
			headers:     []sip.Header{{Name: "Max-Forwards", Value: "0"}},
			wantHeaders: []sip.Header{{Name: "Max-Forwards", Value: "0"}},
			wantErr:     ErrTooManyHops,
		},
		"non-numeric is rejected": {
			headers:     []sip.Header{{Name: "Max-Forwards", Value: "abc"}},
			wantHeaders: []sip.Header{{Name: "Max-Forwards", Value: "abc"}},
			wantErr:     ErrInvalidMaxForwards,
		},
		"negative is rejected": {
			headers:     []sip.Header{{Name: "Max-Forwards", Value: "-1"}},
			wantHeaders: []sip.Header{{Name: "Max-Forwards", Value: "-1"}},
			wantErr:     ErrInvalidMaxForwards,
		},
		"above 255 is rejected": {
			headers:     []sip.Header{{Name: "Max-Forwards", Value: "256"}},
			wantHeaders: []sip.Header{{Name: "Max-Forwards", Value: "256"}},
			wantErr:     ErrInvalidMaxForwards,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req := &sip.Message{IsRequest: true, Method: "INVITE", Headers: tc.headers}

			gotErr := DecrementMaxForwards(req)

			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("DecrementMaxForwards() error = %v, want %v", gotErr, tc.wantErr)
			}
			if !reflect.DeepEqual(req.Headers, tc.wantHeaders) {
				t.Errorf("Headers = %+v, want %+v", req.Headers, tc.wantHeaders)
			}
		})
	}
}

func TestAddVia(t *testing.T) {
	req := &sip.Message{IsRequest: true, Method: "INVITE", Headers: []sip.Header{
		{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
		{Name: "Call-ID", Value: "abc"},
	}}

	AddVia(req, "proxy.example.com:5060", "z9hG4bKproxy")

	want := []sip.Header{
		{Name: "Via", Value: "SIP/2.0/UDP proxy.example.com:5060;branch=z9hG4bKproxy"},
		{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
		{Name: "Call-ID", Value: "abc"},
	}
	if !reflect.DeepEqual(req.Headers, want) {
		t.Errorf("Headers = %+v, want %+v", req.Headers, want)
	}
	if branch := sip.ViaBranch(req.Headers[0].Value); branch != "z9hG4bKproxy" {
		t.Errorf("ViaBranch(top Via) = %q, want %q", branch, "z9hG4bKproxy")
	}
}
