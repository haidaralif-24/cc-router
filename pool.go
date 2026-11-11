package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type KeyState struct {
	Provider      *Provider
	ID            string // safe log label, e.g. "openrouter[0]"
	Key           string
	Status        string // "active", "cooling", "disabled"
	CooldownUntil time.Time
	Fails         int
	LastUsed      time.Time
}

type Pool struct {
	mu              sync.Mutex
	keys            []*KeyState
	defaultCooldown time.Duration
}

func NewPool(cfg *Config) *Pool {
	p := &Pool{
		defaultCooldown: time.Duration(cfg.DefaultCooldownSeconds) * time.Second,
	}
	for i := range cfg.Providers {
		prov := &cfg.Providers[i]
		for i, k := range prov.Keys {
			p.keys = append(p.keys, &KeyState{
				Provider: prov,
				ID:       fmt.Sprintf("%s[%d]", prov.Name, i),
				Key:      k,
				Status:   "active",
			})
		}
	}
	return p
}

func (p *Pool) Candidates() []*KeyState {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	var out []*KeyState

	for _, k := range p.keys {
		if k.Status == "cooling" && now.After(k.CooldownUntil) {
			k.Status = "active"
			k.Fails = 0
		}
		if k.Status == "active" {
			out = append(out, k)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider.Tier != out[j].Provider.Tier {
			return out[i].Provider.Tier < out[j].Provider.Tier
		}
		return out[i].LastUsed.Before(out[j].LastUsed)
	})

	// Reserve the head of the list while still holding the lock. Without this,
	// concurrent requests would all observe the same LastUsed and pick the same
	// key, defeating round-robin.
	if len(out) > 0 {
		out[0].LastUsed = now
	}

	return out
}

func (p *Pool) Fail(k *KeyState, status int, retryAfter time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	k.Fails++
	switch {
	case status == 401 || status == 403:
		k.Status = "disabled"
	case status == 429:
		k.Status = "cooling"
		cd := retryAfter
		if cd == 0 {
			cd = time.Duration(k.Provider.CooldownSeconds) * time.Second
			if cd == 0 {
				cd = p.defaultCooldown
			}
		}
		k.CooldownUntil = time.Now().Add(cd)
	case status >= 500 || status == 0:
		if k.Fails >= 3 {
			k.Status = "cooling"
			k.CooldownUntil = time.Now().Add(p.defaultCooldown)
		}
	}
}

func (p *Pool) Success(k *KeyState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k.Fails = 0
	k.LastUsed = time.Now()
}
