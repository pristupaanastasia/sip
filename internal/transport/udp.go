package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

// MaxDatagram is the largest value of the 16-bit UDP Length field (RFC 768).
// Real payloads are a bit smaller (65507 bytes over IPv4, 65527 over IPv6),
// so a buffer of this size can hold any datagram. A smaller buffer would make
// ReadFromUDP silently drop the rest of a larger message. RFC 3261 18.1.1 asks
// senders to switch to TCP above 1300 bytes, but a receiver may still get more.
const MaxDatagram = 65535

type Handler func(data []byte, from *net.UDPAddr)

func Listen(addr string) (*net.UDPConn, error) {
	ua, resolveErr := net.ResolveUDPAddr("udp", addr)
	if resolveErr != nil {
		return nil, fmt.Errorf("resolve listen address %q: %w", addr, resolveErr)
	}
	return net.ListenUDP("udp", ua)
}

func Serve(ctx context.Context, conn *net.UDPConn, handle Handler) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, MaxDatagram)
	for {
		n, from, readErr := conn.ReadFromUDP(buf)
		if errors.Is(readErr, net.ErrClosed) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read udp: %w", readErr)
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		wg.Add(1)
		go func(data []byte, from *net.UDPAddr) {
			defer wg.Done()
			handle(data, from)
		}(data, from)
	}
}
