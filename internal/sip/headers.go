package sip

import "strings"

var compactForms = map[string]string{
	"c": "Content-Type",
	"e": "Content-Encoding",
	"f": "From",
	"i": "Call-ID",
	"k": "Supported",
	"l": "Content-Length",
	"m": "Contact",
	"s": "Subject",
	"t": "To",
	"v": "Via",
}

var irregularNames = map[string]string{
	"call-id":          "Call-ID",
	"cseq":             "CSeq",
	"mime-version":     "MIME-Version",
	"www-authenticate": "WWW-Authenticate",
}

func CanonicalName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if full, ok := compactForms[lower]; ok {
		return full
	}
	if irregular, ok := irregularNames[lower]; ok {
		return irregular
	}

	words := strings.Split(lower, "-")
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, "-")
}
