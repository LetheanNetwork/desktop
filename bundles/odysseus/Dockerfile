# lthn/odysseus — the Odysseus AI workspace as ONE sealed image for the
# lthn/desktop bundle. Run Apple-native via go-container; inference stays
# host-side (the model is reached over the host bridge, never baked in).
#
# Odysseus's own compose splits three services — the app, ChromaDB, SearXNG.
# We co-locate them under supervisord so the bundle is a single OCI image, the
# way opencode's lthn/dev bakes opencode-ai. Build context = this directory.
#
# ITERATION FLAGS — resolve against a real `container build`:
#   - SearXNG packaging + run command is the fiddliest piece; verify against
#     the current searxng release (the run module + settings format drift).
#   - Chroma runs as an in-image server on :8000 (CHROMADB_HOST=localhost),
#     matching the compose's HTTP-client mode but co-located.
#   - v1 runs services as root inside the VM (the VM is the isolation
#     boundary). Add the PUID/PGID drop for defence-in-depth later.

FROM python:3.12-slim

# Odysseus's own system deps + supervisord + git for the source clones.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake curl git nodejs npm tmux openssh-client gosu \
      supervisor \
  && rm -rf /var/lib/apt/lists/*

# --- Odysseus app -------------------------------------------------------
WORKDIR /app
# TODO(pin): pin to a commit/tag before shipping; main HEAD for the first build.
RUN git clone --depth 1 https://github.com/pewdiepie-archdaemon/odysseus.git /app \
  && pip install --no-cache-dir -r /app/requirements.txt

# --- ChromaDB (vector store, in-image server on :8000) ------------------
RUN pip install --no-cache-dir chromadb

# --- SearXNG (Deep Research's search backend, on :8080) -----------------
# Run from source (PYTHONPATH=/opt/searxng), installing its requirements — NOT
# an editable build: `-e` fails because searxng's PEP-517 build step imports
# msgspec, which isn't present. The searx package is imported in place; supervisord
# runs `python -m searx.webapp` from /opt/searxng.
RUN git clone --depth 1 https://github.com/searxng/searxng.git /opt/searxng \
  && pip install --no-cache-dir -r /opt/searxng/requirements.txt
COPY searxng-settings.yml /etc/searxng/settings.yml

# --- Process supervision ------------------------------------------------
COPY supervisord.conf /etc/supervisor/conf.d/odysseus.conf
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENV APP_BIND=0.0.0.0 \
    APP_PORT=7000 \
    CHROMADB_HOST=localhost \
    SEARXNG_INSTANCE=http://localhost:8080 \
    SEARXNG_SETTINGS_PATH=/etc/searxng/settings.yml

EXPOSE 7000

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["supervisord", "-n", "-c", "/etc/supervisor/conf.d/odysseus.conf"]
