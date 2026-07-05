package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/handlers"
	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/mail"
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

	// PublicBaseURL is randoread's externally visible URL, including the
	// Traefik path prefix (e.g. "https://howapped.zapto.org/randoread/").
	// Needed to build absolute asset URLs for email images, since an email
	// client has no <base> tag or session to resolve relative ones against.
	PublicBaseURL string

	EmailFrom string
	EmailTo   string
	EmailUser string
	EmailPass string
	SMTPHost  string
	SMTPPort  string
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
	dropboxConnect := handlers.NewDropboxConnect(dropboxClient, cfg.DropboxRedirectURI, cfg.PublicBaseURL)
	mux.HandleFunc("GET /api/dropbox/auth", dropboxConnect.HandleAuth)
	mux.HandleFunc("GET /api/dropbox/callback", dropboxConnect.HandleCallback)
	mux.HandleFunc("GET /api/dropbox/status", dropboxConnect.HandleStatus)
	mux.HandleFunc("POST /api/dropbox/disconnect", dropboxConnect.HandleDisconnect)

	dailyHandler := handlers.NewDailyHandler(dropboxClient, cfg.VaultRoot, nil)
	dailyHandler.AuthToken = cfg.AuthToken
	mux.Handle("GET /api/daily", dailyHandler)

	assetHandler := handlers.NewAssetHandler(dropboxClient, cfg.VaultRoot)
	mux.Handle("GET /api/asset", assetHandler)

	// A full recursive vault listing is slow (many paginated Dropbox round
	// trips) and doesn't need to be fresher than this — Rando/Clipped share
	// one cache so it's warmed by whichever gets clicked first.
	vaultListCache := dropbox.NewCachedLister(dropboxClient, vaultListCacheTTL)
	if !testing.Testing() {
		// Avoid firing real Dropbox network calls from unit tests, which
		// build a mux with a fake app key/no tokens via this same path.
		go warmVaultListCache(vaultListCache, cfg.VaultRoot)
	}

	randoHandler := handlers.NewRandoHandler(dropboxClient, vaultListCache, cfg.VaultRoot, nil, nil)
	randoHandler.AuthToken = cfg.AuthToken
	mux.Handle("GET /api/rando", randoHandler)

	clippedHandler := handlers.NewClippedHandler(dropboxClient, vaultListCache, cfg.VaultRoot, nil)
	clippedHandler.AuthToken = cfg.AuthToken
	mux.Handle("GET /api/clipped", clippedHandler)

	smtpConfig := mail.Config{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.EmailUser, Password: cfg.EmailPass}
	sendFunc := func(subject, html string) error {
		return mail.Send(smtpConfig, cfg.EmailFrom, cfg.EmailTo, subject, html)
	}
	emailHandler := handlers.NewEmailHandler(dropboxClient, cfg.VaultRoot, cfg.PublicBaseURL, cfg.AuthToken, sendFunc)
	mux.Handle("POST /api/email", emailHandler)

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

// vaultListCacheTTL controls how long a recursive vault listing is reused
// before Rando/Clipped re-list Dropbox. Doesn't need to be fresher than this
// for a personal vault that changes gradually over a day.
const vaultListCacheTTL = 24 * time.Hour

// warmVaultListCache pre-populates the cache at startup so the very first
// Rando/Clipped click isn't the one paying for a slow recursive listing.
// Best-effort: if Dropbox isn't connected yet, this just fails silently —
// the next real click (or the next server restart) tries again.
func warmVaultListCache(cache *dropbox.CachedLister, vaultRoot string) {
	if _, err := cache.ListFolder(vaultRoot, true); err != nil {
		log.Printf("vault list cache warmup (vault root) failed, will retry on next request: %v", err)
	}
	if _, err := cache.ListFolder(vaultRoot+handlers.ClippingsSubpath, true); err != nil {
		log.Printf("vault list cache warmup (Clippings) failed, will retry on next request: %v", err)
	}
}

const (
	defaultPublicBaseURL = "https://howapped.zapto.org/randoread/"
	defaultEmailTo       = "jon@howapped.com"
	defaultSMTPHost      = "smtp.gmail.com"
	defaultSMTPPort      = "587"
)

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

	publicBaseURL := os.Getenv("PUBLIC_BASE_URL")
	if publicBaseURL == "" {
		publicBaseURL = defaultPublicBaseURL
	}

	emailTo := os.Getenv("EMAIL_TO")
	if emailTo == "" {
		emailTo = defaultEmailTo
	}

	emailUser := os.Getenv("EMAIL_USER")

	emailFrom := os.Getenv("EMAIL_FROM")
	if emailFrom == "" {
		emailFrom = emailUser
	}

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = defaultSMTPHost
	}

	smtpPort := os.Getenv("SMTP_PORT")
	if smtpPort == "" {
		smtpPort = defaultSMTPPort
	}

	return Config{
		AuthToken:          mustEnv("AUTH_TOKEN"),
		AuthTokenIssuedAt:  issuedAt,
		DataDir:            dataDir,
		DropboxAppKey:      os.Getenv("DROPBOX_APP_KEY"),
		DropboxRedirectURI: os.Getenv("DROPBOX_REDIRECT_URI"),
		VaultRoot:          vaultRoot,
		PublicBaseURL:      publicBaseURL,
		EmailFrom:          emailFrom,
		EmailTo:            emailTo,
		EmailUser:          emailUser,
		EmailPass:          os.Getenv("EMAIL_PASS"),
		SMTPHost:           smtpHost,
		SMTPPort:           smtpPort,
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
