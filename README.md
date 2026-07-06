# tools-randoread

A small Go webservice that reads notes from a Dropbox-synced Obsidian vault
(`jw-mind`) and renders them in the browser, styled with a "Dark Moss"
GitHub-like dark theme.

## Features

- **Daily 📅** — fetches today's daily note
  (`periodic/daily/YYYY-MM-DD-[W]WW-ddd.md`). Title shown is the bare
  filename.
- **Rando ♻** — fetches a random note from anywhere in the vault, clickable
  any time. Picks one note per day and keeps serving that same note until
  4pm Europe/London, when the next click picks a new one (persisted to
  survive restarts). Excludes Dropbox "conflicted copy" duplicates,
  Excalidraw drawings (`*.excalidraw.md`), and anything under `templates/`.
  Title shown is the vault-relative path (e.g. `books / 2026 / main`). The
  full recursive vault listing this needs is slow (many paginated Dropbox
  round trips), so it's cached in memory for 24h and shared with Clipped —
  only the first pick after a cold start or cache expiry pays that cost.
- **Rando Clipped 🎠** — same one-per-day/4pm-reset behavior as Rando, but
  candidates come from `Clippings/` only. Gates independently of Rando (its
  own persisted pick). Title still reads `Clippings / name` since it's
  formatted relative to the true vault root, not the Clippings subfolder.
- **Most Recently Clipped ✂️** — fetches the most recently modified article
  from the vault's `Clippings/` folder, any time (shares Rando's listing
  cache and file filters). Rendered with a "Date Clipped: yyyy-mm-dd hh:mm"
  heading (Europe/London time) showing when it was last modified.
- **Email this note** (burger menu, ☰) — emails the currently displayed note
  as an HTML-embedded message to `jon@howapped.com` (configurable), images
  included.
- Obsidian-flavored markdown rendering: relative embeds (`![[file.png]]`,
  `![[note.pdf]]`) resolve by looking the filename up in a full recursive
  vault listing — matching how Obsidian itself resolves them vault-wide,
  not by assuming a fixed folder (needed because not everything lives in
  `assets/`; e.g. tools-browsernotes' reMarkable sync drops handwritten-note
  PDFs in `_remarkable-emails-via-browsernotes/`). Images render as `<img>`;
  PDFs render as an inline `<object>` with a fallback link. Standard
  markdown links/images render normally, bare URLs autolink, GFM
  tables/tasklists supported. Obsidian Web Clipper frontmatter renders as
  "🔗 View original | example.com" (clickable link, then its base domain so
  it's clear where it leads) followed by the article's title as a heading,
  instead of raw YAML.
- No static files are generated anywhere — Daily/Rando/Clipped all fetch and
  render a note's own markdown straight from Dropbox on every request, so
  text edits show up on the very next fetch. The one exception: resolving
  an embed's filename to a path uses the same 24h-cached vault listing as
  Rando/Clipped (see above — a fresh listing takes ~45s, so this is a
  deliberate trade-off). A *brand-new* file referenced by a *brand-new*
  embed may not resolve until that cache refreshes or the server restarts;
  embeds referencing files that already existed in the vault are unaffected.
- Sticky header (all four buttons stay visible, divided from the content
  below by a horizontal rule) with an independently scrolling,
  darker-background reading area. The active section has a white outline,
  and the URL hash (`#daily` / `#rando` / `#rando-clipped` / `#clipped`)
  reflects it — reloading, bookmarking, or sharing the URL restores the same
  view.

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
internal/state/         — Rando's "note of the day" pin (JSON file)
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
