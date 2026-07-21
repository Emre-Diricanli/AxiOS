package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/axios-os/axios/internal/axiosd"
)

func authenticate(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		providedDigest := sha256.Sum256([]byte(provided))
		tokenDigest := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(providedDigest[:], tokenDigest[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func newHandler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", authenticate(token, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("/api/system/stats", authenticate(token, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stats, err := axiosd.GatherSystemStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}))
	return mux
}

func main() {
	listenAddress := flag.String("listen", "127.0.0.1:3001", "address for the telemetry API")
	tokenFile := flag.String("token-file", "", "path to a file containing the bearer token")
	flag.Parse()
	if *tokenFile == "" {
		log.Fatal("-token-file is required")
	}
	tokenBytes, err := os.ReadFile(*tokenFile)
	if err != nil {
		log.Fatalf("read token file: %v", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if len(token) < 32 {
		log.Fatal("telemetry token must contain at least 32 characters")
	}

	server := &http.Server{
		Addr:              *listenAddress,
		Handler:           newHandler(token),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("AxiOS telemetry listening on %s", *listenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
