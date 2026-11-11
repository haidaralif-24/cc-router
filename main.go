package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

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

	log.Printf("Starting router on %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, nil); err != nil {
		log.Fatal(err)
	}
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
