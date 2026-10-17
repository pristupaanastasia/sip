package sip

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		msg     *Message
		wantErr error
	}{
		"valid request": {
			msg: request("INVITE", mandatoryHeaders("1 INVITE")),
		},
		"valid request without Max-Forwards": {
			msg: request("OPTIONS", mandatoryHeaders("7 OPTIONS")),
		},
		"valid response": {
			msg: response(200, mandatoryHeaders("1 INVITE")),
		},
		"CSeq with extra whitespace": {
			msg: request("INVITE", mandatoryHeaders("1   INVITE")),
		},
		"largest allowed CSeq number": {
			msg: request("INVITE", mandatoryHeaders("2147483647 INVITE")),
		},

		"request without Via": {
			msg:     request("INVITE", without(mandatoryHeaders("1 INVITE"), "Via")),
			wantErr: ErrMissingHeader,
		},
		"request without From": {
			msg:     request("INVITE", without(mandatoryHeaders("1 INVITE"), "From")),
			wantErr: ErrMissingHeader,
		},
		"request without To": {
			msg:     request("INVITE", without(mandatoryHeaders("1 INVITE"), "To")),
			wantErr: ErrMissingHeader,
		},
		"request without Call-ID": {
			msg:     request("INVITE", without(mandatoryHeaders("1 INVITE"), "Call-ID")),
			wantErr: ErrMissingHeader,
		},
		"request without CSeq": {
			msg:     request("INVITE", without(mandatoryHeaders("1 INVITE"), "CSeq")),
			wantErr: ErrMissingHeader,
		},
		"response without Via": {
			msg:     response(200, without(mandatoryHeaders("1 INVITE"), "Via")),
			wantErr: ErrMissingHeader,
		},

		"CSeq without method": {
			msg:     request("INVITE", mandatoryHeaders("1")),
			wantErr: ErrInvalidCSeq,
		},
		"CSeq with extra field": {
			msg:     request("INVITE", mandatoryHeaders("1 INVITE extra")),
			wantErr: ErrInvalidCSeq,
		},
		"non-numeric CSeq number": {
			msg:     request("INVITE", mandatoryHeaders("abc INVITE")),
			wantErr: ErrInvalidCSeq,
		},
		"negative CSeq number": {
			msg:     request("INVITE", mandatoryHeaders("-1 INVITE")),
			wantErr: ErrInvalidCSeq,
		},
		"CSeq number above 2^31-1": {
			msg:     request("INVITE", mandatoryHeaders("2147483648 INVITE")),
			wantErr: ErrInvalidCSeq,
		},
		"invalid CSeq in response": {
			msg:     response(200, mandatoryHeaders("abc INVITE")),
			wantErr: ErrInvalidCSeq,
		},

		"CSeq method differs from request method": {
			msg:     request("OPTIONS", mandatoryHeaders("1 INVITE")),
			wantErr: ErrCSeqMethodMismatch,
		},
		"CSeq method differs only in case": {
			msg:     request("INVITE", mandatoryHeaders("1 invite")),
			wantErr: ErrCSeqMethodMismatch,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotErr := Validate(tc.msg)
			if !errors.Is(gotErr, tc.wantErr) {
				t.Errorf("Validate() error = %v, want %v", gotErr, tc.wantErr)
			}
		})
	}
}
