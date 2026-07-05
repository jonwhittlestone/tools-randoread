package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jonwhittlestone/tools-randoread/handlers"
	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/state"
)

//go:embed static
var staticFiles embed.FS

// Config holds the environment-derived settings needed to wire up the mux.
type Config struct {
	AuthToken         string
	AuthTokenIssuedAt time.Time

	// DataDir is where per-install state (currently just the Dropbox token
	// file) is persisted. It should point at a mounted volume in production
	// so it survives container restarts/redeploys.
	DataDir string

	// DropboxAppKey is public (safe to embed) — it's the OAuth client_id,
	// not a secret. Shared with tools-browsernotes' existing app
	// registration since both read the same vault.
	DropboxAppKey      string
	DropboxRedirectURI string

	// VaultRoot is the Dropbox path (from the account root) to the Obsidian
	// vault, e.g. "/DropsyncFiles/jw-mind".
	VaultRoot string
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

	tokenStore := dropbox.NewStore(filepath.Join(cfg.DataDir, "dropbox_tokens.json"))
	dropboxClient := dropbox.NewClient(cfg.DropboxAppKey, tokenStore)
	dropboxConnect := handlers.NewDropboxConnect(dropboxClient, cfg.DropboxRedirectURI)
	mux.HandleFunc("GET /api/dropbox/auth", dropboxConnect.HandleAuth)
	mux.HandleFunc("GET /api/dropbox/callback", dropboxConnect.HandleCallback)
	mux.HandleFunc("GET /api/dropbox/status", dropboxConnect.HandleStatus)
	mux.HandleFunc("POST /api/dropbox/disconnect", dropboxConnect.HandleDisconnect)

	dailyHandler := handlers.NewDailyHandler(dropboxClient, cfg.VaultRoot, nil)
	mux.Handle("GET /api/daily", dailyHandler)

	assetHandler := handlers.NewAssetHandler(dropboxClient, cfg.VaultRoot)
	mux.Handle("GET /api/asset", assetHandler)

	randoStore := state.NewCooldownStore(filepath.Join(cfg.DataDir, "rando_cooldown.json"))
	randoHandler := handlers.NewRandoHandler(dropboxClient, dropboxClient, cfg.VaultRoot, randoStore, nil, nil)
	mux.Handle("GET /api/rando", randoHandler)
	mux.HandleFunc("GET /api/rando/status", randoHandler.HandleStatus)

	clippedStore := state.NewCooldownStore(filepath.Join(cfg.DataDir, "clipped_cooldown.json"))
	clippedHandler := handlers.NewClippedHandler(dropboxClient, dropboxClient, cfg.VaultRoot, clippedStore, nil)
	mux.Handle("GET /api/clipped", clippedHandler)
	mux.HandleFunc("GET /api/clipped/status", clippedHandler.HandleStatus)

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticFS)))

	return auth.RequireToken(mux)
}

// defaultVaultRoot matches tools-browsernotes' DEFAULT_VAULT_ROOT — both
// services read the same Dropbox-synced Obsidian vault.
const defaultVaultRoot = "/DropsyncFiles/jw-mind"

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

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("failed to create DATA_DIR %q: %v", dataDir, err)
	}

	vaultRoot := os.Getenv("VAULT_ROOT")
	if vaultRoot == "" {
		vaultRoot = defaultVaultRoot
	}

	return Config{
		AuthToken:          mustEnv("AUTH_TOKEN"),
		AuthTokenIssuedAt:  issuedAt,
		DataDir:            dataDir,
		DropboxAppKey:      os.Getenv("DROPBOX_APP_KEY"),
		DropboxRedirectURI: os.Getenv("DROPBOX_REDIRECT_URI"),
		VaultRoot:          vaultRoot,
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
