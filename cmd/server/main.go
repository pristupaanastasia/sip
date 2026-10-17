package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/sip"
	"github.com/pristupaanastasia/sip/internal/transport"
)

var errNotResponse = errors.New("file contains a request, expected a response")

type config struct {
	listenAddr   string
	responseFile string
	delay        time.Duration
}

type app struct {
	sender    transport.Sender
	template  *sip.Message
	delay     time.Duration
	tagSecret []byte
	log       *logging.Logger
}

func main() {
	cfg := parseFlags()
	lg := logging.New("server")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	runErr := run(ctx, cfg, lg)
	stop()

	if runErr != nil {
		lg.Fatal("server failed", slog.Any("err", runErr))
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.listenAddr, "listen", "0.0.0.0:5070", "UDP address to listen on")
	flag.StringVar(&cfg.responseFile, "response", "testdata/200ok.txt", "file with the final response")
	flag.DurationVar(&cfg.delay, "delay", time.Second, "delay before the final response")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config, lg *logging.Logger) error {
	template, loadErr := loadResponse(cfg.responseFile)
	if loadErr != nil {
		return loadErr
	}

	conn, listenErr := transport.Listen(cfg.listenAddr)
	if listenErr != nil {
		return listenErr
	}
	defer conn.Close()

	a := &app{
		sender:    transport.Sender{Conn: conn, Log: lg},
		template:  template,
		delay:     cfg.delay,
		tagSecret: newTagSecret(),
		log:       lg,
	}

	lg.Info("listening", slog.String("addr", conn.LocalAddr().String()))
	serveErr := transport.Serve(ctx, conn, a.handle)
	lg.Info("stopped")
	return serveErr
}

func (a *app) handle(data []byte, from *net.UDPAddr) {
	msg, parseErr := sip.Parse(data)
	if parseErr != nil {
		a.log.Warn("drop unparsable message", slog.String("from", from.String()), slog.Any("err", parseErr))
		var shortErr *sip.BodyTooShortError
		if errors.As(parseErr, &shortErr) && shortErr.Message.ExpectsResponse() {
			a.sender.Send(sip.NewResponse(shortErr.Message, 400, "Bad Request"), from)
		}
		return
	}
	a.log.Received(from, msg)

	if validateErr := sip.Validate(msg); validateErr != nil {
		a.log.Warn("drop invalid message", slog.String("from", from.String()), slog.Any("err", validateErr))
		if msg.ExpectsResponse() {
			a.sender.Send(sip.NewResponse(msg, 400, "Bad Request"), from)
		}
		return
	}

	if !msg.ExpectsResponse() {
		return
	}

	a.sender.Send(sip.NewTrying(msg), from)
	time.Sleep(a.delay)
	a.sender.Send(a.finalResponse(msg), from)
}

func (a *app) finalResponse(req *sip.Message) *sip.Message {
	resp := sip.NewResponse(req, a.template.StatusCode, a.template.Reason)
	for _, h := range a.template.Headers {
		switch h.Name {
		case "Via", "From", "To", "Call-ID", "CSeq", "Content-Length":
			continue
		}
		resp.Headers = append(resp.Headers, h)
	}
	resp.Body = a.template.Body

	if to, ok := resp.Get("To"); ok && !strings.Contains(to, ";tag=") {
		resp.Set("To", to+";tag="+a.toTag(req))
	}
	return resp
}

func loadResponse(path string) (*sip.Message, error) {
	raw, readFileErr := os.ReadFile(path)
	if readFileErr != nil {
		return nil, fmt.Errorf("read response file: %w", readFileErr)
	}
	resp, parseErr := sip.ParseFile(raw)
	if parseErr != nil {
		return nil, fmt.Errorf("parse response file %q: %w", path, parseErr)
	}
	if resp.IsRequest {
		return nil, fmt.Errorf("%q: %w", path, errNotResponse)
	}
	return resp, nil
}

func (a *app) toTag(req *sip.Message) string {
	callID, _ := req.Get("Call-ID")
	from, _ := req.Get("From")

	mac := hmac.New(sha256.New, a.tagSecret)
	mac.Write([]byte(callID))
	mac.Write([]byte{0})
	mac.Write([]byte(sip.Param(from, "tag")))
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

func newTagSecret() []byte {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	return secret
}
