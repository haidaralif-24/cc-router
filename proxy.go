package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func (r *Router) tryUpstream(w http.ResponseWriter, in *http.Request, body []byte, ks *KeyState, path string) bool {
	// 1. Handle Model Override
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil && ks.Provider.OverrideModel != "" {
		payload["model"] = ks.Provider.OverrideModel
		body, _ = json.Marshal(payload)
	}

	// 2. Build Request
	upstreamURL := ks.Provider.BaseURL + path
	outReq, _ := http.NewRequestWithContext(in.Context(), "POST", upstreamURL, bytes.NewReader(body))

	// 3. Headers
	outReq.Header.Set("content-type", "application/json")
	outReq.Header.Set("anthropic-version", in.Header.Get("anthropic-version"))
	if b := in.Header.Get("anthropic-beta"); b != "" {
		outReq.Header.Set("anthropic-beta", b)
	}

	// 4. Auth
	if ks.Provider.AuthStyle == "bearer" {
		outReq.Header.Set("Authorization", "Bearer "+ks.Key)
	} else {
		outReq.Header.Set("x-api-key", ks.Key)
	}

	// 5. Extra Headers
	for k, v := range ks.Provider.ExtraHeaders {
		outReq.Header.Set(k, v)
	}

	// 6. Execute
	resp, err := r.client.Do(outReq)
	if err != nil {
		log.Printf("fail %s (%s): network error: %v", ks.Provider.Name, ks.Key[:8], err)
		r.pool.Fail(ks, 0, 0)
		return false
	}
	defer resp.Body.Close()

	// 7. Success (Stream back)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		streamCopy(w, resp.Body)
		r.pool.Success(ks)
		return true
	}

	// 8. Failover logic
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
	log.Printf("fail %s (%s): status %d", ks.Provider.Name, ks.Key[:8], resp.StatusCode)
	r.pool.Fail(ks, resp.StatusCode, retryAfter)
	return false
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		if k == "Content-Length" || k == "Transfer-Encoding" {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func streamCopy(w http.ResponseWriter, src io.Reader) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func parseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	var seconds int
	if _, err := fmt.Sscanf(val, "%d", &seconds); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
