package sip

import "strconv"

type Header struct {
	Name  string
	Value string
}

type Message struct {
	IsRequest bool

	Method     string
	RequestURI string

	StatusCode int
	Reason     string

	Headers []Header
	Body    []byte
}

func (m *Message) StartLine() string {
	if m.IsRequest {
		return m.Method + " " + m.RequestURI + " " + Version
	}
	return Version + " " + strconv.Itoa(m.StatusCode) + " " + m.Reason
}

func (m *Message) Get(name string) (string, bool) {
	name = CanonicalName(name)
	for _, h := range m.Headers {
		if h.Name == name {
			return h.Value, true
		}
	}
	return "", false
}

func (m *Message) Values(name string) []string {
	name = CanonicalName(name)
	var values []string
	for _, h := range m.Headers {
		if h.Name == name {
			values = append(values, h.Value)
		}
	}
	return values
}

func (m *Message) Add(name, value string) {
	m.Headers = append(m.Headers, Header{Name: CanonicalName(name), Value: value})
}

func (m *Message) Set(name, value string) {
	name = CanonicalName(name)
	updated := make([]Header, 0, len(m.Headers)+1)
	replaced := false
	for _, h := range m.Headers {
		if h.Name != name {
			updated = append(updated, h)
			continue
		}
		if !replaced {
			updated = append(updated, Header{Name: name, Value: value})
			replaced = true
		}
	}
	if !replaced {
		updated = append(updated, Header{Name: name, Value: value})
	}
	m.Headers = updated
}

func (m *Message) Del(name string) {
	name = CanonicalName(name)
	kept := make([]Header, 0, len(m.Headers))
	for _, h := range m.Headers {
		if h.Name != name {
			kept = append(kept, h)
		}
	}
	m.Headers = kept
}

func (m *Message) ExpectsResponse() bool {
	return m.IsRequest && m.Method != "ACK"
}

func NewTrying(req *Message) *Message {
	resp := NewResponse(req, 100, "Trying")
	if timestamp, ok := req.Get("Timestamp"); ok {
		resp.Add("Timestamp", timestamp)
	}
	return resp
}

func NewResponse(req *Message, code int, reason string) *Message {
	resp := &Message{StatusCode: code, Reason: reason}
	for _, h := range req.Headers {
		switch h.Name {
		case "Via", "From", "To", "Call-ID", "CSeq":
			resp.Headers = append(resp.Headers, h)
		}
	}
	return resp
}
