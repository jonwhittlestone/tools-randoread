package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jonwhittlestone/tools-randoread/handlers"
)

//go:embed static
var staticFiles embed.FS

// Config holds the environment-derived settings needed to wire up the mux.
type Config struct {
	AuthToken         string
	AuthTokenIssuedAt time.Time
}

// newMux wires up all routes and wraps them in the token-auth middleware.
// RequireToken only guards /api/ routes (see handlers.isPublicPath), so
// wrapping the whole mux is safe: /health and static assets pass through.
func newMux(cfg Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	auth := handlers.NewAuth(cfg.AuthToken, cfg.AuthTokenIssuedAt, nil)
	mux.HandleFunc("GET /api/auth", auth.HandleValidate)

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticFS)))

	return auth.RequireToken(mux)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}

func loadConfig() Config {
	issuedAt, err := time.Parse(time.RFC3339, mustEnv("AUTH_TOKEN_ISSUED_AT"))
	if err != nil {
		log.Fatalf("invalid AUTH_TOKEN_ISSUED_AT: %v", err)
	}
	return Config{
		AuthToken:         mustEnv("AUTH_TOKEN"),
		AuthTokenIssuedAt: issuedAt,
	}
}

func main() {
	cfg := loadConfig()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("randoread listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, newMux(cfg)))
}
