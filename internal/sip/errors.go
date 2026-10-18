package sip

import (
	"errors"
	"fmt"
)

var (
	ErrEmptyMessage       = errors.New("sip: empty message")
	ErrMalformedStartLine = errors.New("sip: malformed start line")
	ErrUnsupportedVersion = errors.New("sip: unsupported SIP version")
	ErrInvalidStatusCode  = errors.New("sip: invalid status code")
	ErrMalformedHeader    = errors.New("sip: malformed header")
	ErrInvalidContentLen  = errors.New("sip: invalid Content-Length")
	ErrBodyTooShort       = errors.New("sip: body shorter than Content-Length")
	ErrMissingHeader      = errors.New("sip: missing mandatory header")
	ErrInvalidCSeq        = errors.New("sip: invalid CSeq")
	ErrCSeqMethodMismatch = errors.New("sip: CSeq method does not match request method")
)

type BodyTooShortError struct {
	Message *Message
	Want    int
	Got     int
}

func (e *BodyTooShortError) Error() string {
	return fmt.Sprintf("%v: header says %d, got %d", ErrBodyTooShort, e.Want, e.Got)
}

func (e *BodyTooShortError) Unwrap() error {
	return ErrBodyTooShort
}
