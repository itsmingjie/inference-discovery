// A deterministic demo endpoint, NOT a real model. Run from any directory:
// go run examples/local-provider/mock.go --listen :8000
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	listen := flag.String("listen", ":8000", "API listen address")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"demo-chat","object":"model"}]}`)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req struct {
			Stream bool   `json:"stream"`
			Model  string `json:"model"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if req.Model != "demo-chat" {
			http.Error(w, "model unavailable", 404)
			return
		}
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Hello from the deterministic demo endpoint."}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, word := range []string{"Hello ", "from ", "the ", "deterministic ", "demo ", "endpoint."} {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": word}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
			f.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(80 * time.Millisecond):
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		f.Flush()
	})
	log.Printf("Deterministic demo API listening on %s (no real model)", *listen)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}
	log.Fatal(server.ListenAndServe())
}
