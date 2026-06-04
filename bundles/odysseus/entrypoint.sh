#!/bin/sh
# Ensure the persisted directories exist before the services start (the
# /app/data volume is mounted by the sandbox), then hand off to supervisord.
#
# v1 runs the services as root inside the contained VM — the Apple VM is the
# isolation boundary. The PUID/PGID privilege drop (Odysseus's own entrypoint
# does this for host bind-mounts) is a defence-in-depth follow-up.
set -e

mkdir -p /app/data /app/data/chroma /app/logs

# SearXNG refuses to start without a real secret_key. Generate one once and
# persist it under the data volume so it's stable across restarts.
SECRET_FILE=/app/data/searxng-secret
if [ ! -f "$SECRET_FILE" ]; then
  head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$SECRET_FILE"
fi
export SEARXNG_SECRET="$(cat "$SECRET_FILE")"

exec "$@"
