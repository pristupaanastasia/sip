package sip

import (
	"reflect"
	"testing"
)

func TestMessagePushVia(t *testing.T) {
	msg := &Message{Headers: []Header{
		{Name: "Via", Value: "client"},
		{Name: "Call-ID", Value: "abc"},
	}}

	msg.PushVia("proxy")

	want := []Header{
		{Name: "Via", Value: "proxy"},
		{Name: "Via", Value: "client"},
		{Name: "Call-ID", Value: "abc"},
	}
	if !reflect.DeepEqual(msg.Headers, want) {
		t.Errorf("Headers = %+v, want %+v", msg.Headers, want)
	}
}

func TestMessagePopVia(t *testing.T) {
	tests := map[string]struct {
		headers     []Header
		wantVia     string
		wantOK      bool
		wantHeaders []Header
	}{
		"separate Via rows": {
			headers:     []Header{{Name: "Via", Value: "proxy"}, {Name: "Via", Value: "client"}, {Name: "Call-ID", Value: "abc"}},
			wantVia:     "proxy",
			wantOK:      true,
			wantHeaders: []Header{{Name: "Via", Value: "client"}, {Name: "Call-ID", Value: "abc"}},
		},
		"comma-separated Via values": {
			headers:     []Header{{Name: "Via", Value: "proxy , client"}, {Name: "Call-ID", Value: "abc"}},
			wantVia:     "proxy",
			wantOK:      true,
			wantHeaders: []Header{{Name: "Via", Value: "client"}, {Name: "Call-ID", Value: "abc"}},
		},
		"Via is not the first header": {
			headers:     []Header{{Name: "Call-ID", Value: "abc"}, {Name: "Via", Value: "proxy"}, {Name: "CSeq", Value: "1 INVITE"}},
			wantVia:     "proxy",
			wantOK:      true,
			wantHeaders: []Header{{Name: "Call-ID", Value: "abc"}, {Name: "CSeq", Value: "1 INVITE"}},
		},
		"no Via": {
			headers:     []Header{{Name: "Call-ID", Value: "abc"}},
			wantVia:     "",
			wantOK:      false,
			wantHeaders: []Header{{Name: "Call-ID", Value: "abc"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			msg := &Message{Headers: tc.headers}
			gotVia, gotOK := msg.PopVia()
			if gotVia != tc.wantVia || gotOK != tc.wantOK {
				t.Errorf("PopVia() = %q, %v, want %q, %v", gotVia, gotOK, tc.wantVia, tc.wantOK)
			}
			if !reflect.DeepEqual(msg.Headers, tc.wantHeaders) {
				t.Errorf("Headers = %+v, want %+v", msg.Headers, tc.wantHeaders)
			}
		})
	}
}

func TestMessagePopViaDoesNotCorruptSharedSlice(t *testing.T) {
	original := []Header{{Name: "Via", Value: "proxy"}, {Name: "Via", Value: "client"}, {Name: "Call-ID", Value: "abc"}}
	snapshot := append([]Header(nil), original...)
	msg := &Message{Headers: original}

	msg.PopVia()

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("PopVia modified the original slice: %+v, want %+v", original, snapshot)
	}
}

func TestViaBranch(t *testing.T) {
	tests := map[string]struct {
		via  string
		want string
	}{
		"branch only":               {via: "SIP/2.0/UDP host:5060;branch=z9hG4bK1", want: "z9hG4bK1"},
		"branch among other params": {via: "SIP/2.0/UDP host;received=1.2.3.4;branch=z9hG4bK2;rport", want: "z9hG4bK2"},
		"spaces around params":      {via: "SIP/2.0/UDP host ; branch=z9hG4bK3", want: "z9hG4bK3"},
		"param name in upper case":  {via: "SIP/2.0/UDP host;BRANCH=z9hG4bK4", want: "z9hG4bK4"},
		"no branch":                 {via: "SIP/2.0/UDP host;rport", want: ""},
		"no params":                 {via: "SIP/2.0/UDP host", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ViaBranch(tc.via); got != tc.want {
				t.Errorf("ViaBranch(%q) = %q, want %q", tc.via, got, tc.want)
			}
		})
	}
}
