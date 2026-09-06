#!/usr/bin/env bash
# Build and run the container. No docker-compose on purpose -- this is the
# whole deployment, and a single explicit `docker run` is easier to audit
# than a compose file that hides the same flags.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

IMAGE_NAME="{{PROJECT_NAME}}"
CONTAINER_NAME="{{PROJECT_NAME}}"
HOST_PORT="${HOST_PORT:-8000}"
DATA_DIR="$(pwd)/data"
SECRET_FILE="$(pwd)/.secret_key"

mkdir -p "$DATA_DIR"

# SECRET_KEY must stay stable across restarts (it signs nothing sensitive
# here, but rotating it invalidates the running rate-limit/session state
# less predictably) -- generate it once and reuse it.
if [ ! -f "$SECRET_FILE" ]; then
  umask 177
  python3 -c "import secrets; print(secrets.token_hex(32))" > "$SECRET_FILE" 2>/dev/null \
    || openssl rand -hex 32 > "$SECRET_FILE"
fi
SECRET_KEY="$(cat "$SECRET_FILE")"

echo "Building $IMAGE_NAME..."
docker build -t "$IMAGE_NAME" .

if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
  echo "Removing existing container $CONTAINER_NAME..."
  docker rm -f "$CONTAINER_NAME" >/dev/null
fi

echo "Starting $CONTAINER_NAME on port $HOST_PORT..."
docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  -p "127.0.0.1:${HOST_PORT}:8000" \
  -v "$DATA_DIR:/app/data" \
  -e "SECRET_KEY=$SECRET_KEY" \
  -e "SECURE_COOKIES=${SECURE_COOKIES:-true}" \
  --read-only \
  --tmpfs /tmp \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --pids-limit 128 \
  --memory 256m \
  "$IMAGE_NAME"

echo "Running. Bootstrap the first admin with:"
echo "  docker exec -it $CONTAINER_NAME python -m scripts.manage bootstrap-admin --name \"Your Name\""
echo ""
echo "Note: the container only publishes to 127.0.0.1. Put a TLS-terminating"
echo "reverse proxy (nginx, Caddy) in front of it for real deployments --"
echo "this app assumes HTTPS is handled upstream and marks cookies Secure"
echo "accordingly."
