package proxy

import (
	"fmt"
	"strconv"

	"github.com/pristupaanastasia/sip/internal/sip"
)

const defaultMaxForwards = 70

func DecrementMaxForwards(req *sip.Message) error {
	value, ok := req.Get("Max-Forwards")
	if !ok {
		req.Set("Max-Forwards", strconv.Itoa(defaultMaxForwards))
		return nil
	}

	hops, atoiErr := strconv.Atoi(value)
	if atoiErr != nil || hops < 0 || hops > 255 {
		return fmt.Errorf("%w: %q", ErrInvalidMaxForwards, value)
	}
	if hops == 0 {
		return ErrTooManyHops
	}
	req.Set("Max-Forwards", strconv.Itoa(hops-1))
	return nil
}

func AddVia(req *sip.Message, host, branch string) {
	req.PushVia("SIP/2.0/UDP " + host + ";branch=" + branch)
}
