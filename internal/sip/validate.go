package sip

import (
	"fmt"
	"strconv"
	"strings"
)

var requiredHeaders = []string{"Via", "From", "To", "Call-ID", "CSeq"}

func Validate(m *Message) error {
	for _, name := range requiredHeaders {
		if _, ok := m.Get(name); !ok {
			return fmt.Errorf("%w: %s", ErrMissingHeader, name)
		}
	}

	cseq, _ := m.Get("CSeq")
	method, cseqErr := parseCSeq(cseq)
	if cseqErr != nil {
		return cseqErr
	}
	if m.IsRequest && method != m.Method {
		return fmt.Errorf("%w: CSeq has %q, request is %q", ErrCSeqMethodMismatch, method, m.Method)
	}
	return nil
}

func parseCSeq(value string) (string, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return "", fmt.Errorf("%w: %q", ErrInvalidCSeq, value)
	}
	if _, parseErr := strconv.ParseUint(fields[0], 10, 31); parseErr != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidCSeq, value)
	}
	return fields[1], nil
}
