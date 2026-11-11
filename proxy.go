package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// isRetryableStatus reports whether a failed upstream response should trigger
// failover to another key. Everything else (notably 400 Bad Request) is
// terminal and must be forwarded to the client unchanged.
func isRetryableStatus(status int) bool {
	switch {
	case status == http.StatusUnauthorized,
		status == http.StatusForbidden,
		status == http.StatusTooManyRequests,
		status >= 500:
		return true
	}
	return false
}

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
		log.Printf("fail %s: network error: %v", ks.ID, err)
		r.pool.Fail(ks, 0, 0)
		return false
	}
	defer resp.Body.Close()

	// 7. Success (Stream back)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if err := streamCopy(w, resp.Body); err != nil {
			// The response is already partially written, so we cannot fail
			// over here, but the key should not be recorded as healthy.
			log.Printf("stream %s failed: %v", ks.ID, err)
			r.pool.Fail(ks, 0, 0)
			return true
		}
		r.pool.Success(ks)
		return true
	}

	// 8. Failover logic
	if isRetryableStatus(resp.StatusCode) {
		log.Printf("fail %s: status %d", ks.ID, resp.StatusCode)
		r.pool.Fail(ks, resp.StatusCode, parseRetryAfter(resp.Header.Get("Retry-After")))
		return false
	}

	// 9. Non-retryable upstream error (e.g. 400 Bad Request): return it
	// verbatim instead of masking it as "all keys exhausted".
	log.Printf("upstream %s: status %d (not retrying)", ks.ID, resp.StatusCode)
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("forwarding error response from %s failed: %v", ks.ID, err)
	}
	return true
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

func streamCopy(w http.ResponseWriter, src io.Reader) error {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return fmt.Errorf("writing to client: %w", werr)
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("reading from upstream: %w", err)
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
