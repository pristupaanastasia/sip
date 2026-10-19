package sip

import "testing"

func TestCanonicalName(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"already canonical":          {input: "Content-Type", want: "Content-Type"},
		"lower case":                 {input: "content-type", want: "Content-Type"},
		"upper case":                 {input: "CONTENT-TYPE", want: "Content-Type"},
		"single word":                {input: "via", want: "Via"},
		"surrounding whitespace":     {input: "  Via\t", want: "Via"},
		"compact Via":                {input: "v", want: "Via"},
		"compact Via upper case":     {input: "V", want: "Via"},
		"compact Call-ID":            {input: "i", want: "Call-ID"},
		"compact From":               {input: "f", want: "From"},
		"compact To":                 {input: "t", want: "To"},
		"compact Content-Length":     {input: "l", want: "Content-Length"},
		"compact Contact":            {input: "m", want: "Contact"},
		"compact Content-Type":       {input: "c", want: "Content-Type"},
		"compact Content-Encoding":   {input: "e", want: "Content-Encoding"},
		"compact Subject":            {input: "s", want: "Subject"},
		"compact Supported":          {input: "k", want: "Supported"},
		"irregular Call-ID":          {input: "call-id", want: "Call-ID"},
		"irregular Call-ID mixed":    {input: "Call-Id", want: "Call-ID"},
		"irregular CSeq":             {input: "CSEQ", want: "CSeq"},
		"irregular WWW-Authenticate": {input: "www-authenticate", want: "WWW-Authenticate"},
		"irregular MIME-Version":     {input: "mime-version", want: "MIME-Version"},
		"unknown extension header":   {input: "x-custom-header", want: "X-Custom-Header"},
		"unknown single letter":      {input: "z", want: "Z"},
		"empty":                      {input: "", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := CanonicalName(tc.input); got != tc.want {
				t.Errorf("CanonicalName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
