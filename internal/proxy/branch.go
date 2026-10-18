package proxy

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/pristupaanastasia/sip/internal/sip"
)

const magicCookie = "z9hG4bK"

func NewSecret() []byte {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	return secret
}

func Branch(secret []byte, req *sip.Message) (string, error) {
	topVia, viaOK := req.Get("Via")
	callID, callIDOK := req.Get("Call-ID")
	cseq, cseqOK := req.Get("CSeq")
	if !viaOK || !callIDOK || !cseqOK {
		return "", ErrMissingTransactionHeader
	}
	topVia, _, _ = strings.Cut(topVia, ",")

	mac := hmac.New(sha256.New, secret)
	for _, part := range []string{strings.TrimSpace(topVia), callID, cseq, req.RequestURI} {
		mac.Write([]byte(part))
		mac.Write([]byte{0})
	}
	return magicCookie + hex.EncodeToString(mac.Sum(nil)[:8]), nil
}
