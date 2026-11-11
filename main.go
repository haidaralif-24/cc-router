package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type Router struct {
	pool   *Pool
	client *http.Client
	token  string
}

func main() {
	cfg, err := LoadConfig("config.jsonc")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	routerToken := os.Getenv("ROUTER_TOKEN")
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
	http.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("Starting router on %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, nil); err != nil {
		log.Fatal(err)
	}
}

func (r *Router) handleMessages(w http.ResponseWriter, req *http.Request) {
	// Auth Check
	authHeader := req.Header.Get("authorization")
	apiKey := req.Header.Get("x-api-key")
	if apiKey != r.token && authHeader != "Bearer "+r.token {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	for _, ks := range r.pool.Candidates() {
		if r.tryUpstream(w, req, body, ks) {
			return
		}
	}

	http.Error(w, "All keys exhausted or cooling down", http.StatusTooManyRequests)
}
