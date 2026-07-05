#!/usr/bin/env bash
# deploy.sh — rsync to doylestonex, build natively on Pi (ARM64), restart via podman
#
# Usage:
#   make deploy
#   # or directly:
#   bash deploy/deploy.sh
#
# Requires:
#   - SSH access to doylestonex configured in ~/.ssh/config
#   - .env present at $REMOTE_DIR on doylestonex (see .env.example)
#   - podman + podman-compose installed on doylestonex

set -euo pipefail

REMOTE_USER="admin"
REMOTE_HOST="doylestonex"
REMOTE_DIR="/home/admin/www/tools-randoread"
TRAEFIK_CONFIG_DIR="/home/admin/traefik/config/dynamic"

echo "==> Testing SSH connection"
ssh "$REMOTE_HOST" "echo 'SSH OK'"

echo "==> Pruning old images on doylestonex"
ssh "$REMOTE_HOST" "podman image prune -f" || true

echo "==> Syncing project to $REMOTE_HOST:$REMOTE_DIR"
rsync -avz --exclude='.git' \
           --exclude='bin/' \
           --exclude='.env' \
           --exclude='data/' \
           ./ "$REMOTE_USER@$REMOTE_HOST:$REMOTE_DIR/"

echo "==> Installing Traefik dynamic config"
scp deploy/randoread-traefik.yml "$REMOTE_USER@$REMOTE_HOST:$TRAEFIK_CONFIG_DIR/randoread.yml"

echo "==> Rebuilding image"
ssh "$REMOTE_HOST" "cd $REMOTE_DIR && podman-compose build"

echo "==> Ensuring data directory exists on $REMOTE_HOST"
ssh "$REMOTE_HOST" "mkdir -p /home/admin/randoread-data"

echo "==> Restarting container with host networking"
ssh "$REMOTE_HOST" "
  podman stop tools-randoread_randoread_1 2>/dev/null || true
  podman rm   tools-randoread_randoread_1 2>/dev/null || true
  sleep 1
  podman run -d \
    --name tools-randoread_randoread_1 \
    --network host \
    --env-file $REMOTE_DIR/.env \
    -e PORT=8085 \
    -v /home/admin/randoread-data:/app/data:Z \
    --restart unless-stopped \
    --health-cmd 'curl -sf http://localhost:8085/health' \
    --health-interval 30s \
    --health-timeout 10s \
    --health-retries 3 \
    --health-start-period 5s \
    localhost/tools-randoread_randoread:latest
"

echo "==> Waiting for health check..."
sleep 5
ssh "$REMOTE_HOST" "curl -sf http://localhost:8085/health" && echo "  -> healthy" || echo "  -> health check FAILED"

echo "==> Deploy complete. App available at https://howapped.zapto.org/randoread/health"
