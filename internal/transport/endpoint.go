package transport

import (
	"errors"
	"log/slog"
	"net"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/sip"
)

type Endpoint struct {
	Conn *net.UDPConn
	Log  *logging.Logger
}

func (e Endpoint) Send(msg *sip.Message, to *net.UDPAddr) {
	if _, sendErr := e.Conn.WriteToUDP(msg.Bytes(), to); sendErr != nil {
		e.Log.Error("send failed", slog.String("to", to.String()), slog.Any("err", sendErr))
		return
	}
	e.Log.Info("sent message", slog.String("to", to.String()), slog.String("start_line", msg.StartLine()))
}

func (e Endpoint) Receive(data []byte, from *net.UDPAddr) (*sip.Message, bool) {
	msg, parseErr := sip.Parse(data)
	if parseErr != nil {
		e.Log.Warn("drop unparsable message", slog.String("from", from.String()), slog.Any("err", parseErr))
		var shortErr *sip.BodyTooShortError
		if errors.As(parseErr, &shortErr) && shortErr.Message.ExpectsResponse() {
			e.Send(sip.NewResponse(shortErr.Message, 400, "Bad Request"), from)
		}
		return nil, false
	}
	e.Log.Received(from, msg)

	if validateErr := sip.Validate(msg); validateErr != nil {
		e.Log.Warn("drop invalid message", slog.String("from", from.String()), slog.Any("err", validateErr))
		if msg.ExpectsResponse() {
			e.Send(sip.NewResponse(msg, 400, "Bad Request"), from)
		}
		return nil, false
	}
	return msg, true
}
