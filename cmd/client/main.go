package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/sip"
	"github.com/pristupaanastasia/sip/internal/transport"
)

const (
	retransmitT1 = 500 * time.Millisecond
	retransmitT2 = 4 * time.Second
)

var (
	errNotRequest      = errors.New("file contains a response, expected a request")
	errNoFinalResponse = errors.New("no final response received")
	errFailureResponse = errors.New("request failed")
	errMissingCallID   = errors.New("request has no Call-ID")
)

type config struct {
	proxyAddr   string
	requestFile string
	timeout     time.Duration
}

func main() {
	cfg := parseFlags()
	lg := logging.New("client")

	if runErr := run(cfg, lg); runErr != nil {
		lg.Fatal("client failed", slog.Any("err", runErr))
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.proxyAddr, "proxy", "127.0.0.1:5060", "address of the SIP proxy")
	flag.StringVar(&cfg.requestFile, "request", "testdata/invite.txt", "file with the SIP request")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "how long to wait for the final response")
	flag.Parse()
	return cfg
}

func run(cfg config, lg *logging.Logger) error {
	req, loadErr := loadRequest(cfg.requestFile)
	if loadErr != nil {
		return loadErr
	}

	to, resolveErr := net.ResolveUDPAddr("udp", cfg.proxyAddr)
	if resolveErr != nil {
		return fmt.Errorf("resolve proxy address %q: %w", cfg.proxyAddr, resolveErr)
	}
	conn, listenErr := net.ListenUDP("udp", nil)
	if listenErr != nil {
		return fmt.Errorf("listen udp: %w", listenErr)
	}
	defer conn.Close()

	resp, requestErr := sendRequest(conn, req, to, cfg.timeout, lg)
	if requestErr != nil {
		return requestErr
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s", errFailureResponse, resp.StartLine())
	}
	return nil
}

func sendRequest(conn *net.UDPConn, req *sip.Message, to *net.UDPAddr, timeout time.Duration, lg *logging.Logger) (*sip.Message, error) {
	raw := req.Bytes()
	callID, ok := req.Get("Call-ID")
	if !ok {
		return nil, errMissingCallID
	}
	deadline := time.Now().Add(timeout)

	retransmitting := true
	retransmitAt := time.Now()
	interval := retransmitT1
	attempt := 0

	buf := make([]byte, transport.MaxDatagram)
	for {
		if retransmitting && !time.Now().Before(retransmitAt) {
			if _, sendErr := conn.WriteToUDP(raw, to); sendErr != nil {
				return nil, fmt.Errorf("send request to %s: %w", to, sendErr)
			}
			attempt++
			lg.Info("sent request", slog.String("to", to.String()), slog.String("start_line", req.StartLine()), slog.Int("attempt", attempt))
			retransmitAt = time.Now().Add(interval)
			interval = nextRetransmitInterval(req.Method, interval)
		}

		readDeadline := deadline
		if retransmitting && retransmitAt.Before(deadline) {
			readDeadline = retransmitAt
		}
		if deadlineErr := conn.SetReadDeadline(readDeadline); deadlineErr != nil {
			return nil, fmt.Errorf("set read deadline: %w", deadlineErr)
		}

		n, from, readErr := conn.ReadFromUDP(buf)
		if errors.Is(readErr, os.ErrDeadlineExceeded) {
			if time.Now().Before(deadline) {
				continue
			}
			return nil, fmt.Errorf("%w within %s", errNoFinalResponse, timeout)
		}
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		resp, parseErr := sip.Parse(buf[:n])
		if parseErr != nil {
			lg.Warn("drop unparsable message", slog.String("from", from.String()), slog.Any("err", parseErr))
			continue
		}
		lg.Received(from, resp)

		if resp.IsRequest {
			lg.Warn("drop unexpected request", slog.String("from", from.String()))
			continue
		}
		if validateErr := sip.Validate(resp); validateErr != nil {
			lg.Warn("drop invalid response", slog.String("from", from.String()), slog.Any("err", validateErr))
			continue
		}
		if respCallID, _ := resp.Get("Call-ID"); respCallID != callID {
			lg.Warn("drop response for another call", slog.String("from", from.String()), slog.String("call_id", respCallID))
			continue
		}

		retransmitting = false
		if resp.StatusCode >= 200 {
			return resp, nil
		}
	}
}

func nextRetransmitInterval(method string, interval time.Duration) time.Duration {
	next := 2 * interval
	if method != "INVITE" && next > retransmitT2 {
		return retransmitT2
	}
	return next
}

func loadRequest(path string) (*sip.Message, error) {
	raw, readFileErr := os.ReadFile(path)
	if readFileErr != nil {
		return nil, fmt.Errorf("read request file: %w", readFileErr)
	}
	req, parseErr := sip.ParseFile(raw)
	if parseErr != nil {
		return nil, fmt.Errorf("parse request file %q: %w", path, parseErr)
	}
	if !req.IsRequest {
		return nil, fmt.Errorf("%q: %w", path, errNotRequest)
	}
	return req, nil
}
