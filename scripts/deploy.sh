#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-$HOME/bimmlite}"
BRANCH="${BRANCH:-main}"
SERVICE="${SERVICE:-bimmlite-backend}"
LOG_FILE="${LOG_FILE:-/var/log/bimmlite-deploy.log}"
NGINX_ROOT="${NGINX_ROOT:-}"

log() {
  local level="$1"
  shift
  local message="$*"
  local ts
  ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  printf '%s [%s] %s\n' "$ts" "$level" "$message" | tee -a "$LOG_FILE"
}

fail() {
  log "ERROR" "$*"
  exit 1
}

ensure_logfile() {
  sudo touch "$LOG_FILE"
  sudo chown "$(id -un)":"$(id -gn)" "$LOG_FILE"
  sudo chmod 664 "$LOG_FILE"
}

detect_nginx_root() {
  if [[ -n "$NGINX_ROOT" ]]; then
    printf '%s\n' "$NGINX_ROOT"
    return 0
  fi

  local root
  root="$(sudo nginx -T 2>/dev/null | awk '
    $1 == "root" && $2 ~ /^\// {
      gsub(";", "", $2);
      print $2;
      exit
    }
  ')"
  if [[ -z "$root" ]]; then
    root="$(sudo grep -R -m1 -oE 'root[[:space:]]+/[^;]+' /etc/nginx/sites-enabled /etc/nginx/conf.d 2>/dev/null | awk '{print $2}')"
  fi
  if [[ -z "$root" ]]; then
    fail "unable to detect NGINX_ROOT"
  fi
  printf '%s\n' "$root"
}

sync_nginx_static() {
  local root="$1"
  local dist_dir="$APP_DIR/frontend/dist"
  [[ -d "$dist_dir" ]] || fail "frontend dist not found: $dist_dir"
  [[ -d "$root" ]] || fail "nginx root does not exist: $root"

  sudo rsync -a --delete "$dist_dir"/ "$root"/
  sudo chown -R caddy:caddy "$root"
  sudo find "$root" -type d -exec chmod 755 {} +
  sudo find "$root" -type f -exec chmod 644 {} +
}

ensure_nginx_cache_rules() {
  local site_file
  site_file="$(sudo grep -R -l -m1 "server_name 34.44.19.28" /etc/nginx/sites-enabled 2>/dev/null || true)"
  [[ -n "$site_file" ]] || return 0

  if sudo grep -q "location = /index.html" "$site_file" && sudo grep -q "location ^~ /assets/" "$site_file"; then
    return 0
  fi

  local tmp
  tmp="$(mktemp)"
  python3 - "$site_file" "$tmp" <<'PY'
from pathlib import Path
import sys

src = Path(sys.argv[1]).read_text()

needle = "    location / {\n        try_files $uri /index.html;\n    }\n"
replacement = """    location = /index.html {\n        add_header Cache-Control \"no-cache, no-store, must-revalidate\" always;\n        expires off;\n        try_files $uri =404;\n    }\n\n    location ^~ /assets/ {\n        add_header Cache-Control \"public, max-age=31536000, immutable\" always;\n        try_files $uri =404;\n    }\n\n    location / {\n        try_files $uri /index.html;\n    }\n"""
if needle in src:
    src = src.replace(needle, replacement)
Path(sys.argv[2]).write_text(src)
PY
  sudo cp "$tmp" "$site_file"
  rm -f "$tmp"
}

build_backend() {
  cd "$APP_DIR"
  if [[ ! -d .venv ]]; then
    python3 -m venv .venv
  fi
  # shellcheck disable=SC1091
  . .venv/bin/activate
  python -m pip install --upgrade pip
  pip install -e backend
  cd backend
  alembic -c alembic.ini upgrade head
  sudo systemctl restart "$SERVICE"
}

build_frontend() {
  cd "$APP_DIR/frontend"
  npm ci
  npm run build
}

healthcheck() {
  local response
  response="$(curl -fsSk https://127.0.0.1/health)"
  printf '%s\n' "$response"
  if ! printf '%s' "$response" | grep -q '"status":"ok"'; then
    fail "healthcheck failed: status ok not found"
  fi
}

main() {
  ensure_logfile
  log "INFO" "deploy start app_dir=$APP_DIR branch=$BRANCH service=$SERVICE"

  cd "$APP_DIR"
  git fetch origin
  git checkout "$BRANCH"
  git reset --hard "origin/$BRANCH"

  build_backend
  build_frontend

  NGINX_ROOT="$(detect_nginx_root)"
  log "INFO" "detected nginx root=$NGINX_ROOT"
  sync_nginx_static "$NGINX_ROOT"
  ensure_nginx_cache_rules

  sudo nginx -t
  sudo systemctl reload nginx

  healthcheck
  log "INFO" "deploy complete"
}

main "$@"
