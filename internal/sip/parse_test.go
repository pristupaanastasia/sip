package sip

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		input   string
		want    *Message
		wantErr error
	}{
		"request with CRLF": {
			input: "INVITE sip:bob@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com;branch=z9hG4bK1\r\n" +
				"Call-ID: abc\r\n" +
				"Content-Length: 4\r\n" +
				"\r\n" +
				"v=0\n",
			want: &Message{
				IsRequest:  true,
				Method:     "INVITE",
				RequestURI: "sip:bob@example.com",
				Headers: []Header{
					{Name: "Via", Value: "SIP/2.0/UDP client.example.com;branch=z9hG4bK1"},
					{Name: "Call-ID", Value: "abc"},
					{Name: "Content-Length", Value: "4"},
				},
				Body: []byte("v=0\n"),
			},
		},
		"response with reason containing spaces": {
			input: "SIP/2.0 487 Request Terminated\r\nCall-ID: abc\r\n\r\n",
			want: &Message{
				StatusCode: 487,
				Reason:     "Request Terminated",
				Headers:    []Header{{Name: "Call-ID", Value: "abc"}},
			},
		},
		"response without reason": {
			input: "SIP/2.0 200\r\n\r\n",
			want:  &Message{StatusCode: 200, Headers: []Header{}},
		},
		"LF line endings": {
			input: "OPTIONS sip:bob@example.com SIP/2.0\nCall-ID: abc\n\n",
			want: &Message{
				IsRequest:  true,
				Method:     "OPTIONS",
				RequestURI: "sip:bob@example.com",
				Headers:    []Header{{Name: "Call-ID", Value: "abc"}},
			},
		},
		"leading empty lines are ignored": {
			input: "\r\n\r\nSIP/2.0 100 Trying\r\n\r\n",
			want:  &Message{StatusCode: 100, Reason: "Trying", Headers: []Header{}},
		},
		"compact forms and mixed case names": {
			input: "SIP/2.0 200 OK\r\n" +
				"v: SIP/2.0/UDP a\r\n" +
				"I: abc\r\n" +
				"cseq: 1 INVITE\r\n" +
				"MAX-FORWARDS: 70\r\n" +
				"\r\n",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers: []Header{
					{Name: "Via", Value: "SIP/2.0/UDP a"},
					{Name: "Call-ID", Value: "abc"},
					{Name: "CSeq", Value: "1 INVITE"},
					{Name: "Max-Forwards", Value: "70"},
				},
			},
		},
		"repeated headers keep their order": {
			input: "SIP/2.0 200 OK\r\n" +
				"Via: SIP/2.0/UDP first\r\n" +
				"Call-ID: abc\r\n" +
				"Via: SIP/2.0/UDP second, SIP/2.0/UDP third\r\n" +
				"\r\n",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers: []Header{
					{Name: "Via", Value: "SIP/2.0/UDP first"},
					{Name: "Call-ID", Value: "abc"},
					{Name: "Via", Value: "SIP/2.0/UDP second, SIP/2.0/UDP third"},
				},
			},
		},
		"folded header value": {
			input: "SIP/2.0 200 OK\r\n" +
				"Subject: long\r\n" +
				" \tsubject\r\n" +
				"\r\n",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []Header{{Name: "Subject", Value: "long subject"}},
			},
		},
		"whitespace around colon": {
			input: "SIP/2.0 200 OK\r\nCall-ID  :   abc  \r\n\r\n",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []Header{{Name: "Call-ID", Value: "abc"}},
			},
		},
		"body longer than Content-Length is truncated": {
			input: "SIP/2.0 200 OK\r\nContent-Length: 3\r\n\r\nabcdef",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []Header{{Name: "Content-Length", Value: "3"}},
				Body:       []byte("abc"),
			},
		},
		"body without Content-Length runs to the end": {
			input: "SIP/2.0 200 OK\r\n\r\nabcdef",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []Header{},
				Body:       []byte("abcdef"),
			},
		},
		"missing empty line after headers": {
			input: "SIP/2.0 200 OK\r\nCall-ID: abc",
			want: &Message{
				StatusCode: 200,
				Reason:     "OK",
				Headers:    []Header{{Name: "Call-ID", Value: "abc"}},
			},
		},

		"empty input": {
			input:   "",
			wantErr: ErrEmptyMessage,
		},
		"only line breaks": {
			input:   "\r\n\r\n",
			wantErr: ErrEmptyMessage,
		},
		"request line without version": {
			input:   "INVITE sip:bob@example.com\r\n\r\n",
			wantErr: ErrMalformedStartLine,
		},
		"request line with extra parts": {
			input:   "INVITE sip:bob@example.com SIP/2.0 extra\r\n\r\n",
			wantErr: ErrMalformedStartLine,
		},
		"status line without code": {
			input:   "SIP/2.0\r\n\r\n",
			wantErr: ErrMalformedStartLine,
		},
		"unsupported request version": {
			input:   "INVITE sip:bob@example.com SIP/3.0\r\n\r\n",
			wantErr: ErrUnsupportedVersion,
		},
		"unsupported response version": {
			input:   "SIP/1.0 200 OK\r\n\r\n",
			wantErr: ErrUnsupportedVersion,
		},
		"non-numeric status code": {
			input:   "SIP/2.0 abc OK\r\n\r\n",
			wantErr: ErrInvalidStatusCode,
		},
		"status code with four digits": {
			input:   "SIP/2.0 2000 OK\r\n\r\n",
			wantErr: ErrInvalidStatusCode,
		},
		"status code below 100": {
			input:   "SIP/2.0 099 OK\r\n\r\n",
			wantErr: ErrInvalidStatusCode,
		},
		"header without colon": {
			input:   "SIP/2.0 200 OK\r\nCall-ID abc\r\n\r\n",
			wantErr: ErrMalformedHeader,
		},
		"header with empty name": {
			input:   "SIP/2.0 200 OK\r\n: abc\r\n\r\n",
			wantErr: ErrMalformedHeader,
		},
		"header name with space inside": {
			input:   "SIP/2.0 200 OK\r\nCall ID: abc\r\n\r\n",
			wantErr: ErrMalformedHeader,
		},
		"continuation line before any header": {
			input:   "SIP/2.0 200 OK\r\n continued\r\n\r\n",
			wantErr: ErrMalformedHeader,
		},
		"non-numeric Content-Length": {
			input:   "SIP/2.0 200 OK\r\nContent-Length: abc\r\n\r\n",
			wantErr: ErrInvalidContentLen,
		},
		"negative Content-Length": {
			input:   "SIP/2.0 200 OK\r\nContent-Length: -1\r\n\r\n",
			wantErr: ErrInvalidContentLen,
		},
		"body shorter than Content-Length": {
			input:   "SIP/2.0 200 OK\r\nl: 10\r\n\r\nabc",
			wantErr: ErrBodyTooShort,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := Parse([]byte(tc.input))

			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("Parse() error = %v, want %v", gotErr, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse() =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	tests := map[string]struct {
		input    string
		wantBody string
	}{
		"LF file with Content-Length counted for CRLF": {
			input:    "SIP/2.0 200 OK\nContent-Length: 8\n\nv=0\ns=-\n",
			wantBody: "v=0\r\ns=-\r\n",
		},
		"Content-Length larger than the body is ignored": {
			input:    "SIP/2.0 200 OK\r\nContent-Length: 999\r\n\r\nv=0\r\n",
			wantBody: "v=0\r\n",
		},
		"Content-Length smaller than the body is ignored": {
			input:    "SIP/2.0 200 OK\r\nContent-Length: 1\r\n\r\nv=0\r\n",
			wantBody: "v=0\r\n",
		},
		"trailing newline after headers is not a body": {
			input:    "SIP/2.0 200 OK\r\nContent-Length: 0\r\n\r\n\n",
			wantBody: "",
		},
		"mixed line endings": {
			input:    "SIP/2.0 200 OK\r\nCall-ID: abc\n\r\nv=0\n",
			wantBody: "v=0\r\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := ParseFile([]byte(tc.input))
			if gotErr != nil {
				t.Fatalf("ParseFile() error = %v", gotErr)
			}
			if string(got.Body) != tc.wantBody {
				t.Errorf("Body = %q, want %q", got.Body, tc.wantBody)
			}
		})
	}
}

func TestParseFileTestdataWithLF(t *testing.T) {
	for _, name := range []string{"invite.txt", "200ok.txt"} {
		t.Run(name, func(t *testing.T) {
			original := readTestdata(t, name)
			withLF := strings.ReplaceAll(original, "\r\n", "\n")

			want, wantErr := Parse([]byte(original))
			if wantErr != nil {
				t.Fatal(wantErr)
			}
			got, gotErr := ParseFile([]byte(withLF))
			if gotErr != nil {
				t.Fatalf("ParseFile() error = %v", gotErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ParseFile(LF) =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}
