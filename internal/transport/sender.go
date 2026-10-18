package transport

import (
	"log/slog"
	"net"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/sip"
)

type Sender struct {
	Conn *net.UDPConn
	Log  *logging.Logger
}

func (s Sender) Send(msg *sip.Message, to *net.UDPAddr) {
	if _, sendErr := s.Conn.WriteToUDP(msg.Bytes(), to); sendErr != nil {
		s.Log.Error("send failed", slog.String("to", to.String()), slog.Any("err", sendErr))
		return
	}
	s.Log.Info("sent message", slog.String("to", to.String()), slog.String("start_line", msg.StartLine()))
}
