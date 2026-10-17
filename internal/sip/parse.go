package sip

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const Version = "SIP/2.0"

func Parse(data []byte) (*Message, error) {
	return parse(data, true)
}

func ParseFile(data []byte) (*Message, error) {
	return parse(normalizeLineEndings(data), false)
}

func parse(data []byte, enforceContentLength bool) (*Message, error) {
	data = bytes.TrimLeft(data, "\r\n")
	if len(data) == 0 {
		return nil, ErrEmptyMessage
	}

	lines, body := splitHead(data)

	msg := &Message{}
	if startLineErr := parseStartLine(msg, lines[0]); startLineErr != nil {
		return nil, startLineErr
	}

	headers, headersErr := parseHeaders(lines[1:])
	if headersErr != nil {
		return nil, headersErr
	}
	msg.Headers = headers

	if enforceContentLength {
		var bodyErr error
		body, bodyErr = applyContentLength(msg, body)
		if bodyErr != nil {
			return nil, bodyErr
		}
	} else if len(bytes.Trim(body, "\r\n")) == 0 {
		body = nil
	}
	if len(body) > 0 {
		msg.Body = make([]byte, len(body))
		copy(msg.Body, body)
	}
	return msg, nil
}

func splitHead(data []byte) (lines []string, body []byte) {
	rest := data
	for len(rest) > 0 {
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			lines = append(lines, string(bytes.TrimSuffix(rest, []byte("\r"))))
			return lines, nil
		}
		line := bytes.TrimSuffix(rest[:end], []byte("\r"))
		rest = rest[end+1:]
		if len(line) == 0 {
			return lines, rest
		}
		lines = append(lines, string(line))
	}
	return lines, nil
}

func parseStartLine(msg *Message, line string) error {
	if len(line) >= 4 && strings.EqualFold(line[:4], "SIP/") {
		return parseStatusLine(msg, line)
	}
	return parseRequestLine(msg, line)
}

func parseStatusLine(msg *Message, line string) error {
	version, rest, ok := strings.Cut(line, " ")
	if !ok {
		return fmt.Errorf("%w: %q", ErrMalformedStartLine, line)
	}
	if !strings.EqualFold(version, Version) {
		return fmt.Errorf("%w: %q", ErrUnsupportedVersion, version)
	}

	codeText, reason, _ := strings.Cut(rest, " ")
	code, atoiErr := strconv.Atoi(codeText)
	if atoiErr != nil || len(codeText) != 3 || code < 100 || code > 699 {
		return fmt.Errorf("%w: %q", ErrInvalidStatusCode, codeText)
	}

	msg.StatusCode = code
	msg.Reason = strings.TrimSpace(reason)
	return nil
}

func parseRequestLine(msg *Message, line string) error {
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return fmt.Errorf("%w: %q", ErrMalformedStartLine, line)
	}
	if !strings.EqualFold(parts[2], Version) {
		return fmt.Errorf("%w: %q", ErrUnsupportedVersion, parts[2])
	}

	msg.IsRequest = true
	msg.Method = parts[0]
	msg.RequestURI = parts[1]
	return nil
}

func parseHeaders(lines []string) ([]Header, error) {
	headers := make([]Header, 0, len(lines))
	for _, line := range lines {
		if line[0] == ' ' || line[0] == '\t' {
			if len(headers) == 0 {
				return nil, fmt.Errorf("%w: continuation without header: %q", ErrMalformedHeader, line)
			}
			last := &headers[len(headers)-1]
			last.Value += " " + strings.TrimSpace(line)
			continue
		}

		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			return nil, fmt.Errorf("%w: %q", ErrMalformedHeader, line)
		}
		headers = append(headers, Header{
			Name:  CanonicalName(name),
			Value: strings.TrimSpace(value),
		})
	}
	return headers, nil
}

func applyContentLength(msg *Message, body []byte) ([]byte, error) {
	value, ok := msg.Get("Content-Length")
	if !ok {
		return body, nil
	}

	length, atoiErr := strconv.Atoi(value)
	if atoiErr != nil || length < 0 {
		return nil, fmt.Errorf("%w: %q", ErrInvalidContentLen, value)
	}
	if length > len(body) {
		return nil, &BodyTooShortError{Message: msg, Want: length, Got: len(body)}
	}
	return body[:length], nil
}

func normalizeLineEndings(data []byte) []byte {
	lf := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
}
