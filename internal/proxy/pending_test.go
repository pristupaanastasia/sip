package proxy

import (
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

var (
	clientA = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1001}
	clientB = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1002}
)

func TestPending(t *testing.T) {
	tests := map[string]struct {
		prepare    func(p *Pending)
		branch     string
		wantClient *net.UDPAddr
		wantOK     bool
	}{
		"returns stored client": {
			prepare:    func(p *Pending) { p.Add("b1", clientA) },
			branch:     "b1",
			wantClient: clientA,
			wantOK:     true,
		},
		"keeps branches apart": {
			prepare: func(p *Pending) {
				p.Add("b1", clientA)
				p.Add("b2", clientB)
			},
			branch:     "b2",
			wantClient: clientB,
			wantOK:     true,
		},
		"unknown branch": {
			prepare: func(p *Pending) { p.Add("b1", clientA) },
			branch:  "other",
		},
		"deleted branch": {
			prepare: func(p *Pending) {
				p.Add("b1", clientA)
				p.Delete("b1")
			},
			branch: "b1",
		},
		"deleting unknown branch is a no-op": {
			prepare: func(p *Pending) {
				p.Add("b1", clientA)
				p.Delete("other")
			},
			branch:     "b1",
			wantClient: clientA,
			wantOK:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := NewPending(time.Minute)
			tc.prepare(p)

			gotClient, gotOK := p.Get(tc.branch)
			if gotClient != tc.wantClient || gotOK != tc.wantOK {
				t.Errorf("Get(%q) = %v, %v, want %v, %v", tc.branch, gotClient, gotOK, tc.wantClient, tc.wantOK)
			}
		})
	}
}

func TestPendingAddReportsDuplicates(t *testing.T) {
	tests := map[string]struct {
		ttl     time.Duration
		wait    time.Duration
		wantNew bool
	}{
		"live entry is a duplicate":   {ttl: time.Minute, wantNew: false},
		"expired entry can be reused": {ttl: 20 * time.Millisecond, wait: 50 * time.Millisecond, wantNew: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := NewPending(tc.ttl)
			if !p.Add("b1", clientA) {
				t.Fatal("first Add() = false, want true")
			}
			time.Sleep(tc.wait)

			if got := p.Add("b1", clientB); got != tc.wantNew {
				t.Errorf("second Add() = %v, want %v", got, tc.wantNew)
			}
			wantClient := clientA
			if tc.wantNew {
				wantClient = clientB
			}
			if client, _ := p.Get("b1"); client != wantClient {
				t.Errorf("Get() = %v, want %v", client, wantClient)
			}
		})
	}
}

func TestPendingExpiry(t *testing.T) {
	p := NewPending(20 * time.Millisecond)
	p.Add("old", clientA)

	time.Sleep(50 * time.Millisecond)

	if _, ok := p.Get("old"); ok {
		t.Errorf("Get(old) found an expired entry")
	}

	p.Add("new", clientB)
	p.mu.Lock()
	_, oldStillStored := p.entries["old"]
	p.mu.Unlock()
	if oldStillStored {
		t.Errorf("Add did not remove the expired entry")
	}
}

func TestPendingConcurrentAccess(t *testing.T) {
	p := NewPending(time.Minute)
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func(branch string) {
			defer wg.Done()
			p.Add(branch, clientA)
			p.Get(branch)
			p.Delete(branch)
		}(strconv.Itoa(i))
	}
	wg.Wait()
}
