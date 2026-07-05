# tools-randoread

A small Go webservice that reads notes from a Dropbox-synced Obsidian vault
(`jw-mind`) and renders them in the browser, styled with a "Dark Moss"
GitHub-like dark theme.

## Features

- **Daily 📅** — fetches today's daily note
  (`periodic/daily/YYYY-MM-DD-[W]WW-ddd.md`). Title shown is the bare
  filename.
- **Rando ♻** — fetches a random note from anywhere in the vault, any time.
  Title shown is the vault-relative path (e.g. `books / 2026 / main`). The
  full recursive vault listing this needs is slow (many paginated Dropbox
  round trips), so it's cached in memory for 24h and shared with Clipped —
  only the first click after a cold start or cache expiry pays that cost.
- **Clipped ✂️** — fetches the most recently modified article from the
  vault's `Clippings/` folder, any time (shares Rando's listing cache).
- **Email this note** (burger menu, ☰) — emails the currently displayed note
  as an HTML-embedded message to `jon@howapped.com` (configurable), images
  included.
- Obsidian-flavored markdown rendering: relative image embeds
  (`![[file.png]]`) resolve against the vault's `assets/` folder, standard
  markdown links/images render normally, bare URLs autolink, GFM
  tables/tasklists supported, and Obsidian Web Clipper frontmatter (`source:`
  field) becomes a clickable "View original" link instead of raw YAML.
- Sticky header (Daily/Rando/Clipped stay visible) with an independently
  scrolling, darker-background reading area below it.

## Usage

Visit `https://howapped.zapto.org/randoread` — you'll see a login screen
unless already authenticated.

To log in, visit:

```
https://howapped.zapto.org/randoread/?token=<token>
```

The token is stored in browser localStorage and is valid for 90 days from
issue. See `04 Development` in `main-randoread.md` for the current login
link.

## Connecting Dropbox

Dropbox access is self-service — no token files need to be placed on the
server. Open the burger menu (☰) → "Connect Dropbox" and authorize access;
the service stores a refresh token on the server (`data/dropbox_tokens.json`)
and renews it automatically. "Disconnect Dropbox" clears it.

This reuses tools-browsernotes' existing Dropbox app registration (same
vault, same app key) — only a redirect URI needs to be added once in the
[Dropbox App Console](https://www.dropbox.com/developers/apps):
`https://howapped.zapto.org/randoread/api/dropbox/callback`.

## Local development

Requires Go 1.22+.

```bash
cp .env.example .env   # fill in AUTH_TOKEN etc.
make run               # starts on :8080 (PORT env var to override)
make test              # go test ./...
```

This repo lives in a Dropbox-synced folder. Avoid running `make build`
from a Dropbox-synced checkout of this repo — the compiled binary would get
synced across machines. It's fine from CI or the copy rsynced to
doylestonex during deploy.

### Architecture

```
main.go                — mux wiring, go:embed static/, env config
handlers/               — one file per HTTP concern (auth, daily, rando,
                          clipped, asset, email, dropbox connect)
internal/dropbox/       — Dropbox HTTP API client (OAuth2+PKCE, download,
                          list_folder), no third-party SDK
internal/markdown/      — goldmark-based renderer + Obsidian preprocessing
internal/note/          — vault path/title formatting
internal/mail/          — SMTP sending
static/                 — vanilla JS/CSS single-page app, no build step
```

Single third-party Go dependency: `goldmark` (pure Go, no CGO). Everything
else is stdlib.

## Deploying

```bash
make deploy   # rsyncs to doylestonex, builds+restarts via podman, updates Traefik
```

Requires SSH access to `doylestonex` (see `~/.ssh/config`) and a `.env` file
already present at `/home/admin/www/tools-randoread/.env` on the host.

## Environment variables

See `.env.example`. Notable ones:

- `AUTH_TOKEN` / `AUTH_TOKEN_ISSUED_AT` — the login token and when it was
  issued (90-day expiry computed from this).
- `DROPBOX_APP_KEY` / `DROPBOX_REDIRECT_URI` — public OAuth client ID and
  callback URL (see "Connecting Dropbox" above).
- `VAULT_ROOT` — Dropbox path to the vault (defaults to
  `/DropsyncFiles/jw-mind`).
- `PUBLIC_BASE_URL` — randoread's externally visible URL, used to build
  absolute image URLs in emails.
- `EMAIL_USER` / `EMAIL_PASS` / `SMTP_HOST` / `SMTP_PORT` / `EMAIL_TO` /
  `EMAIL_FROM` — outbound mail settings (Gmail app password by default).
- `DATA_DIR` — where the Dropbox token file persists (mounted volume in
  production).
