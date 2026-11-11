package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultListenAddr = "127.0.0.1:8787"

type Router struct {
	pool   *Pool
	client *http.Client
	token  string
}

func main() {
	configPath := os.Getenv("CC_ROUTER_CONFIG")
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to determine user home directory: %v", err)
		}
		configPath = filepath.Join(home, ".cc-router", "config.json")
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	listenAddr, err := normalizeListenAddr(cfg.Listen)
	if err != nil {
		log.Fatalf("Invalid listen address: %v", err)
	}

	routerToken := cfg.RouterToken
	if routerToken == "" {
		routerToken = os.Getenv("ROUTER_TOKEN")
	}
	if routerToken == "" {
		log.Fatal("ROUTER_TOKEN environment variable is required")
	}

	r := &Router{
		pool:  NewPool(cfg),
		token: routerToken,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       90 * time.Second,
			},
		},
	}

	http.HandleFunc("/v1/messages", r.handleMessages)
	http.HandleFunc("/v1/messages/count_tokens", r.handleCountTokens)
	http.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("Starting router on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}

// normalizeListenAddr enforces the local-only constraint from the spec. An
// empty listen value defaults to loopback, an empty host (":8787") is bound to
// loopback, and any explicit non-loopback host is rejected.
func normalizeListenAddr(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return defaultListenAddr, nil
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("expected host:port, got %q: %w", addr, err)
	}
	if port == "" {
		return "", fmt.Errorf("missing port in %q", addr)
	}

	if host == "" {
		// ":8787" would bind every interface; force loopback instead.
		host = "127.0.0.1"
	} else if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("refusing to bind to non-loopback host %q: the router holds API keys and must stay localhost-only", host)
		}
	}

	return net.JoinHostPort(host, port), nil
}

func (r *Router) handleMessages(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(w, req) {
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	for _, ks := range r.pool.Candidates() {
		if r.tryUpstream(w, req, body, ks, "/v1/messages") {
			return
		}
	}

	http.Error(w, "All keys exhausted or cooling down", http.StatusTooManyRequests)
}

func (r *Router) handleCountTokens(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(w, req) {
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	for _, ks := range r.pool.Candidates() {
		if r.tryUpstream(w, req, body, ks, "/v1/messages/count_tokens") {
			return
		}
	}

	http.Error(w, "All keys exhausted or cooling down", http.StatusTooManyRequests)
}

func (r *Router) authorized(w http.ResponseWriter, req *http.Request) bool {
	authHeader := req.Header.Get("authorization")
	apiKey := req.Header.Get("x-api-key")
	if apiKey != r.token && authHeader != "Bearer "+r.token {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}
