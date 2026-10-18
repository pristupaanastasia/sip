package proxy

import (
	"net"
	"sync"
	"time"
)

type pendingEntry struct {
	client  *net.UDPAddr
	expires time.Time
}

type Pending struct {
	mu        sync.Mutex
	entries   map[string]pendingEntry
	ttl       time.Duration
	lastSweep time.Time
}

func NewPending(ttl time.Duration) *Pending {
	return &Pending{entries: make(map[string]pendingEntry), ttl: ttl, lastSweep: time.Now()}
}

func (p *Pending) Add(branch string, client *net.UDPAddr) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	p.sweep(now)
	if entry, ok := p.entries[branch]; ok && now.Before(entry.expires) {
		return false
	}
	p.entries[branch] = pendingEntry{client: client, expires: now.Add(p.ttl)}
	return true
}

func (p *Pending) Get(branch string) (*net.UDPAddr, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.entries[branch]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.client, true
}

func (p *Pending) Delete(branch string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, branch)
}

func (p *Pending) sweep(now time.Time) {
	if now.Sub(p.lastSweep) < p.ttl {
		return
	}
	for key, entry := range p.entries {
		if now.After(entry.expires) {
			delete(p.entries, key)
		}
	}
	p.lastSweep = now
}
