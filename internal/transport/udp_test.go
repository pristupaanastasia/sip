package transport

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestListen(t *testing.T) {
	tests := map[string]struct {
		addr    string
		wantErr bool
	}{
		"loopback with random port": {addr: "127.0.0.1:0"},
		"invalid port":              {addr: "127.0.0.1:99999", wantErr: true},
		"missing port":              {addr: "127.0.0.1", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			conn, listenErr := Listen(tc.addr)
			if (listenErr != nil) != tc.wantErr {
				t.Fatalf("Listen(%q) error = %v, wantErr %v", tc.addr, listenErr, tc.wantErr)
			}
			if conn != nil {
				conn.Close()
			}
		})
	}
}

func TestServeDeliversDatagrams(t *testing.T) {
	conn := listenLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type datagram struct {
		data string
		from string
	}
	received := make(chan datagram, 2)
	go func() {
		_ = Serve(ctx, conn, func(data []byte, from *net.UDPAddr) {
			received <- datagram{data: string(data), from: from.String()}
		})
	}()

	sender := dialLoopback(t, conn)
	for _, payload := range []string{"first", "second"} {
		send(t, sender, payload)
	}

	got := map[string]bool{}
	for range 2 {
		select {
		case d := <-received:
			if d.from != sender.LocalAddr().String() {
				t.Errorf("from = %s, want %s", d.from, sender.LocalAddr())
			}
			got[d.data] = true
		case <-time.After(time.Second):
			t.Fatalf("timed out, received %v", got)
		}
	}
	if !got["first"] || !got["second"] {
		t.Errorf("received %v, want first and second", got)
	}
}

func TestServeHandlesDatagramsConcurrently(t *testing.T) {
	conn := listenLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := make(chan struct{})
	started := make(chan struct{}, 2)
	go func() {
		_ = Serve(ctx, conn, func([]byte, *net.UDPAddr) {
			started <- struct{}{}
			<-release
		})
	}()

	sender := dialLoopback(t, conn)
	send(t, sender, "a")
	send(t, sender, "b")

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("second handler did not start while the first one was blocked")
		}
	}
	close(release)
}

func TestServeStopsAndWaitsForHandlers(t *testing.T) {
	conn := listenLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())

	var finished atomic.Bool
	handlerStarted := make(chan struct{})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- Serve(ctx, conn, func([]byte, *net.UDPAddr) {
			close(handlerStarted)
			time.Sleep(100 * time.Millisecond)
			finished.Store(true)
		})
	}()

	send(t, dialLoopback(t, conn), "x")
	<-handlerStarted
	cancel()

	select {
	case serveErr := <-serveDone:
		if serveErr != nil {
			t.Errorf("Serve() error = %v, want nil", serveErr)
		}
		if !finished.Load() {
			t.Error("Serve() returned before the handler finished")
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not return after cancel")
	}
}

func listenLoopback(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, listenErr := Listen("127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("Listen: %v", listenErr)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func dialLoopback(t *testing.T, conn *net.UDPConn) *net.UDPConn {
	t.Helper()
	sender, dialErr := net.DialUDP("udp", nil, conn.LocalAddr().(*net.UDPAddr))
	if dialErr != nil {
		t.Fatalf("DialUDP: %v", dialErr)
	}
	t.Cleanup(func() { sender.Close() })
	return sender
}

func send(t *testing.T, sender *net.UDPConn, payload string) {
	t.Helper()
	if _, writeErr := sender.Write([]byte(payload)); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
}
