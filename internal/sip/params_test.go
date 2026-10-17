package sip

import "testing"

func TestParam(t *testing.T) {
	tests := map[string]struct {
		value string
		name  string
		want  string
	}{
		"tag in From":                    {value: "Alice <sip:alice@example.com>;tag=1928301774", name: "tag", want: "1928301774"},
		"param name is case-insensitive": {value: "<sip:alice@example.com>;TAG=abc", name: "tag", want: "abc"},
		"spaces around param":            {value: "<sip:alice@example.com> ; tag = abc ", name: "tag", want: "abc"},
		"URI params are ignored":         {value: "<sip:alice@example.com;tag=uri>;tag=header", name: "tag", want: "header"},
		"only URI param":                 {value: "<sip:alice@example.com;tag=uri>", name: "tag", want: ""},
		"param without value":            {value: "SIP/2.0/UDP host;rport", name: "rport", want: ""},
		"missing param":                  {value: "<sip:bob@example.com>", name: "tag", want: ""},
		"Via branch":                     {value: "SIP/2.0/UDP host;branch=z9hG4bK1", name: "branch", want: "z9hG4bK1"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Param(tc.value, tc.name); got != tc.want {
				t.Errorf("Param(%q, %q) = %q, want %q", tc.value, tc.name, got, tc.want)
			}
		})
	}
}
