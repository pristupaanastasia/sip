package sip

import "strings"

func (m *Message) PushVia(value string) {
	m.Headers = append([]Header{{Name: "Via", Value: value}}, m.Headers...)
}

func (m *Message) PopVia() (string, bool) {
	for i, h := range m.Headers {
		if h.Name != "Via" {
			continue
		}

		top, rest, combined := strings.Cut(h.Value, ",")
		if combined {
			m.Headers[i].Value = strings.TrimSpace(rest)
		} else {
			m.Headers = append(m.Headers[:i:i], m.Headers[i+1:]...)
		}
		return strings.TrimSpace(top), true
	}
	return "", false
}

func ViaBranch(via string) string {
	return Param(via, "branch")
}
