package sip

import (
	"bytes"
	"strconv"
)

func (m *Message) Bytes() []byte {
	var buf bytes.Buffer
	buf.WriteString(m.StartLine())
	buf.WriteString("\r\n")

	contentLength := strconv.Itoa(len(m.Body))
	wroteContentLength := false
	for _, h := range m.Headers {
		value := h.Value
		if h.Name == "Content-Length" {
			if wroteContentLength {
				continue
			}
			value = contentLength
			wroteContentLength = true
		}
		writeHeader(&buf, h.Name, value)
	}
	if !wroteContentLength {
		writeHeader(&buf, "Content-Length", contentLength)
	}

	buf.WriteString("\r\n")
	buf.Write(m.Body)
	return buf.Bytes()
}

func writeHeader(buf *bytes.Buffer, name, value string) {
	buf.WriteString(name)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}
