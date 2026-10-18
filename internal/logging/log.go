package logging

import (
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/pristupaanastasia/sip/internal/sip"
)

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

type Logger struct {
	*slog.Logger
	out io.Writer
}

func New(component string) *Logger {
	return NewWithWriter(component, os.Stdout)
}

func NewWithWriter(component string, w io.Writer) *Logger {
	out := &lockedWriter{w: w}
	h := slog.NewTextHandler(out, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.StringValue(a.Value.Time().UTC().Format("2006-01-02T15:04:05.000Z"))
			}
			return a
		},
	})
	return &Logger{Logger: slog.New(h).With("component", component), out: out}
}

func (l *Logger) Fatal(msg string, args ...any) {
	l.Error(msg, args...)
	os.Exit(1)
}

func (l *Logger) Received(from net.Addr, m *sip.Message) {
	l.Info("received message", slog.String("from", from.String()))
	_, _ = io.WriteString(l.out, FormatReceived(m))
}

func FormatReceived(m *sip.Message) string {
	var b strings.Builder
	if m.IsRequest {
		b.WriteString("Received request:\n")
		b.WriteString(m.RequestURI)
	} else {
		b.WriteString("Received response:\n")
		b.WriteString(m.StartLine())
	}

	b.WriteString("\nHeaders: {")
	for i, h := range m.Headers {
		if i > 0 {
			b.WriteString(", ")
		}
		writeQuoted(&b, h.Name)
		b.WriteString(": ")
		writeQuoted(&b, h.Value)
	}
	b.WriteString("}\n")
	return b.String()
}

func writeQuoted(b *strings.Builder, s string) {
	b.WriteByte('\'')
	for _, r := range s {
		if r == '\'' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
}
