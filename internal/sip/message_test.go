package sip

import (
	"reflect"
	"testing"
)

func TestMessageStartLine(t *testing.T) {
	tests := map[string]struct {
		msg  *Message
		want string
	}{
		"request": {
			msg:  &Message{IsRequest: true, Method: "INVITE", RequestURI: "sip:bob@example.com"},
			want: "INVITE sip:bob@example.com SIP/2.0",
		},
		"response": {
			msg:  &Message{StatusCode: 200, Reason: "OK"},
			want: "SIP/2.0 200 OK",
		},
		"response with multi-word reason": {
			msg:  &Message{StatusCode: 483, Reason: "Too Many Hops"},
			want: "SIP/2.0 483 Too Many Hops",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.msg.StartLine(); got != tc.want {
				t.Errorf("StartLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMessageGet(t *testing.T) {
	msg := &Message{Headers: []Header{
		{Name: "Via", Value: "first"},
		{Name: "Call-ID", Value: "abc"},
		{Name: "Via", Value: "second"},
	}}

	tests := map[string]struct {
		name   string
		want   string
		wantOK bool
	}{
		"returns first of repeated headers": {name: "Via", want: "first", wantOK: true},
		"case-insensitive lookup":           {name: "call-id", want: "abc", wantOK: true},
		"compact form lookup":               {name: "i", want: "abc", wantOK: true},
		"missing header":                    {name: "CSeq", want: "", wantOK: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotOK := msg.Get(tc.name)
			if got != tc.want || gotOK != tc.wantOK {
				t.Errorf("Get(%q) = %q, %v, want %q, %v", tc.name, got, gotOK, tc.want, tc.wantOK)
			}
		})
	}
}

func TestMessageValues(t *testing.T) {
	msg := &Message{Headers: []Header{
		{Name: "Via", Value: "first"},
		{Name: "Call-ID", Value: "abc"},
		{Name: "Via", Value: "second, third"},
	}}

	tests := map[string]struct {
		name string
		want []string
	}{
		"repeated header in order": {name: "via", want: []string{"first", "second, third"}},
		"single header":            {name: "Call-ID", want: []string{"abc"}},
		"missing header":           {name: "CSeq", want: nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := msg.Values(tc.name); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Values(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestMessageAdd(t *testing.T) {
	msg := &Message{Headers: []Header{{Name: "Via", Value: "first"}}}

	msg.Add("v", "second")

	want := []Header{{Name: "Via", Value: "first"}, {Name: "Via", Value: "second"}}
	if !reflect.DeepEqual(msg.Headers, want) {
		t.Errorf("Headers = %+v, want %+v", msg.Headers, want)
	}
}

func TestMessageSet(t *testing.T) {
	tests := map[string]struct {
		headers []Header
		name    string
		value   string
		want    []Header
	}{
		"replaces in place": {
			headers: []Header{{Name: "Via", Value: "v"}, {Name: "Max-Forwards", Value: "70"}, {Name: "Call-ID", Value: "abc"}},
			name:    "Max-Forwards",
			value:   "69",
			want:    []Header{{Name: "Via", Value: "v"}, {Name: "Max-Forwards", Value: "69"}, {Name: "Call-ID", Value: "abc"}},
		},
		"removes duplicates after the first": {
			headers: []Header{{Name: "To", Value: "a"}, {Name: "Call-ID", Value: "abc"}, {Name: "To", Value: "b"}},
			name:    "to",
			value:   "c",
			want:    []Header{{Name: "To", Value: "c"}, {Name: "Call-ID", Value: "abc"}},
		},
		"appends missing header": {
			headers: []Header{{Name: "Via", Value: "v"}},
			name:    "max-forwards",
			value:   "70",
			want:    []Header{{Name: "Via", Value: "v"}, {Name: "Max-Forwards", Value: "70"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			msg := &Message{Headers: tc.headers}
			msg.Set(tc.name, tc.value)
			if !reflect.DeepEqual(msg.Headers, tc.want) {
				t.Errorf("Headers = %+v, want %+v", msg.Headers, tc.want)
			}
		})
	}
}

func TestMessageDel(t *testing.T) {
	tests := map[string]struct {
		headers []Header
		name    string
		want    []Header
	}{
		"removes all occurrences": {
			headers: []Header{{Name: "Via", Value: "a"}, {Name: "Call-ID", Value: "abc"}, {Name: "Via", Value: "b"}},
			name:    "v",
			want:    []Header{{Name: "Call-ID", Value: "abc"}},
		},
		"missing header leaves message unchanged": {
			headers: []Header{{Name: "Call-ID", Value: "abc"}},
			name:    "Via",
			want:    []Header{{Name: "Call-ID", Value: "abc"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			msg := &Message{Headers: tc.headers}
			msg.Del(tc.name)
			if !reflect.DeepEqual(msg.Headers, tc.want) {
				t.Errorf("Headers = %+v, want %+v", msg.Headers, tc.want)
			}
		})
	}
}

func TestNewResponse(t *testing.T) {
	req := request("INVITE", []Header{
		{Name: "Via", Value: "SIP/2.0/UDP proxy;branch=z9hG4bK2"},
		{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
		{Name: "Max-Forwards", Value: "69"},
		{Name: "To", Value: "<sip:bob@example.com>"},
		{Name: "From", Value: "<sip:alice@example.com>;tag=1"},
		{Name: "Call-ID", Value: "abc"},
		{Name: "CSeq", Value: "1 INVITE"},
		{Name: "Contact", Value: "<sip:alice@client>"},
		{Name: "Content-Type", Value: "application/sdp"},
	})
	req.Body = []byte("v=0\r\n")

	got := NewResponse(req, 100, "Trying")

	want := &Message{
		StatusCode: 100,
		Reason:     "Trying",
		Headers: []Header{
			{Name: "Via", Value: "SIP/2.0/UDP proxy;branch=z9hG4bK2"},
			{Name: "Via", Value: "SIP/2.0/UDP client;branch=z9hG4bK1"},
			{Name: "To", Value: "<sip:bob@example.com>"},
			{Name: "From", Value: "<sip:alice@example.com>;tag=1"},
			{Name: "Call-ID", Value: "abc"},
			{Name: "CSeq", Value: "1 INVITE"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewResponse() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMessageExpectsResponse(t *testing.T) {
	tests := map[string]struct {
		msg  *Message
		want bool
	}{
		"INVITE":   {msg: &Message{IsRequest: true, Method: "INVITE"}, want: true},
		"OPTIONS":  {msg: &Message{IsRequest: true, Method: "OPTIONS"}, want: true},
		"ACK":      {msg: &Message{IsRequest: true, Method: "ACK"}, want: false},
		"response": {msg: &Message{StatusCode: 200}, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.msg.ExpectsResponse(); got != tc.want {
				t.Errorf("ExpectsResponse() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewTrying(t *testing.T) {
	tests := map[string]struct {
		headers []Header
		want    []Header
	}{
		"copies Timestamp": {
			headers: append(mandatoryHeaders("1 INVITE"), Header{Name: "Timestamp", Value: "54.3"}),
			want:    append(mandatoryHeaders("1 INVITE"), Header{Name: "Timestamp", Value: "54.3"}),
		},
		"without Timestamp": {
			headers: mandatoryHeaders("1 INVITE"),
			want:    mandatoryHeaders("1 INVITE"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := NewTrying(request("INVITE", tc.headers))
			if got.StatusCode != 100 || got.Reason != "Trying" {
				t.Errorf("status = %d %s, want 100 Trying", got.StatusCode, got.Reason)
			}
			if !reflect.DeepEqual(got.Headers, tc.want) {
				t.Errorf("Headers = %+v, want %+v", got.Headers, tc.want)
			}
		})
	}
}
