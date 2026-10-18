package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pristupaanastasia/sip/internal/logging"
	"github.com/pristupaanastasia/sip/internal/proxy"
	"github.com/pristupaanastasia/sip/internal/sip"
	"github.com/pristupaanastasia/sip/internal/transport"
)

type config struct {
	listenAddr     string
	upstreamAddr   string
	viaHost        string
	transactionTTL time.Duration
}

type app struct {
	endpoint     transport.Endpoint
	upstream     *net.UDPAddr
	viaHost      string
	branchSecret []byte
	pending      *proxy.Pending
	log          *logging.Logger
}

func main() {
	cfg := parseFlags()
	lg := logging.New("proxy")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	runErr := run(ctx, cfg, lg)
	stop()

	if runErr != nil {
		lg.Fatal("proxy failed", slog.Any("err", runErr))
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.listenAddr, "listen", "0.0.0.0:5060", "UDP address to listen on")
	flag.StringVar(&cfg.upstreamAddr, "upstream", "127.0.0.1:5070", "address of the SIP server")
	flag.StringVar(&cfg.viaHost, "via-host", "127.0.0.1:5060", "host:port the proxy puts into its Via header")
	flag.DurationVar(&cfg.transactionTTL, "transaction-ttl", 32*time.Second, "how long to remember where a forwarded request came from")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config, lg *logging.Logger) error {
	upstream, resolveErr := net.ResolveUDPAddr("udp", cfg.upstreamAddr)
	if resolveErr != nil {
		return fmt.Errorf("resolve upstream address %q: %w", cfg.upstreamAddr, resolveErr)
	}
	conn, listenErr := transport.Listen(cfg.listenAddr)
	if listenErr != nil {
		return listenErr
	}
	defer conn.Close()

	a := &app{
		endpoint:     transport.Endpoint{Conn: conn, Log: lg},
		upstream:     upstream,
		viaHost:      cfg.viaHost,
		branchSecret: proxy.NewSecret(),
		pending:      proxy.NewPending(cfg.transactionTTL),
		log:          lg,
	}

	lg.Info("listening", slog.String("addr", conn.LocalAddr().String()), slog.String("upstream", upstream.String()))
	serveErr := transport.Serve(ctx, conn, a.handle)
	lg.Info("stopped")
	return serveErr
}

func (a *app) handle(data []byte, from *net.UDPAddr) {
	msg, ok := a.endpoint.Receive(data, from)
	if !ok {
		return
	}

	if msg.IsRequest {
		a.handleRequest(msg, from)
		return
	}
	a.handleResponse(msg)
}

func (a *app) handleRequest(req *sip.Message, from *net.UDPAddr) {
	if hopsErr := proxy.DecrementMaxForwards(req); hopsErr != nil {
		a.reject(req, from, hopsErr)
		return
	}

	branch, branchErr := proxy.Branch(a.branchSecret, req)
	if branchErr != nil {
		a.reject(req, from, branchErr)
		return
	}

	if req.ExpectsResponse() {
		a.endpoint.Send(sip.NewTrying(req), from)
		if !a.pending.Add(branch, from) {
			a.log.Info("absorbed retransmission", slog.String("branch", branch))
			return
		}
	}

	proxy.AddVia(req, a.viaHost, branch)
	a.endpoint.Send(req, a.upstream)
}

func (a *app) handleResponse(resp *sip.Message) {
	topVia, ok := resp.PopVia()
	if !ok {
		a.log.Warn("drop response without Via")
		return
	}

	branch := sip.ViaBranch(topVia)
	client, ok := a.pending.Get(branch)
	if !ok {
		a.log.Warn("drop response for unknown transaction", slog.String("branch", branch))
		return
	}

	if resp.StatusCode == 100 {
		return
	}
	if resp.StatusCode >= 200 {
		a.pending.Delete(branch)
	}
	a.endpoint.Send(resp, client)
}

func (a *app) reject(req *sip.Message, from *net.UDPAddr, reason error) {
	a.log.Warn("reject request", slog.String("from", from.String()), slog.Any("err", reason))
	if req.ExpectsResponse() {
		a.endpoint.Send(rejectResponse(req, reason), from)
	}
}

func rejectResponse(req *sip.Message, reason error) *sip.Message {
	if errors.Is(reason, proxy.ErrTooManyHops) {
		return sip.NewResponse(req, 483, "Too Many Hops")
	}
	return sip.NewResponse(req, 400, "Bad Request")
}
