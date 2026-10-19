package sip

func mandatoryHeaders(cseq string) []Header {
	return []Header{
		{Name: "Via", Value: "SIP/2.0/UDP client.example.com;branch=z9hG4bK1"},
		{Name: "From", Value: "<sip:alice@example.com>;tag=1"},
		{Name: "To", Value: "<sip:bob@example.com>"},
		{Name: "Call-ID", Value: "abc"},
		{Name: "CSeq", Value: cseq},
	}
}

func without(headers []Header, name string) []Header {
	kept := make([]Header, 0, len(headers))
	for _, h := range headers {
		if h.Name != name {
			kept = append(kept, h)
		}
	}
	return kept
}

func request(method string, headers []Header) *Message {
	return &Message{IsRequest: true, Method: method, RequestURI: "sip:bob@example.com", Headers: headers}
}

func response(code int, headers []Header) *Message {
	return &Message{StatusCode: code, Reason: "OK", Headers: headers}
}
