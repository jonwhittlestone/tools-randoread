# tools-randoread

A small Go webservice that reads notes from a Dropbox-synced Obsidian vault
(`jw-mind`) and renders them in the browser.

Two modes:
- **Daily** — fetches today's daily note (`periodic/daily/YYYY-MM-DD-[W]WW-ddd.md`).
- **Rando ♻** — fetches a random note from anywhere in the vault. Limited to
  once every 24 hours.

Notes render as styled HTML (a "Dark Moss" GitHub-like dark theme), with
relative Obsidian image embeds (`![[file.png]]`) and standard markdown
links/images rendered inline. A burger menu lets you email the currently
open note as an HTML embed.

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

## Connecting Dropbox

Dropbox access is self-service — no token files need to be placed on the
server. Open the burger menu → "Connect Dropbox" and authorize access; the
service stores a refresh token on the server and renews it automatically.

## Deploying

```bash
make deploy   # rsyncs to doylestonex, builds+restarts via podman, updates Traefik
```

Requires SSH access to `doylestonex` (see `~/.ssh/config`) and a `.env` file
already present at `/home/admin/www/tools-randoread/.env` on the host.

## Environment variables

See `.env.example`.
