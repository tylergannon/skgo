# skgo's self-hosted Linux runners on bob. The image carries what every job
# would otherwise download: Lightpanda for the client handoff check, and mise
# with example/mise.toml's toolset, which mise-action finds instead of
# restoring its cache. A pin that moves past the image still works, since the
# job installs it, until deploy-bob-runners.sh rebuilds the image.
# Build context: the repository root.
FROM ghcr.io/actions/actions-runner:2.338.0
USER root
RUN apt-get update && apt-get install --yes zsh build-essential pkg-config lsof gh curl unzip \
    && userdel ubuntu \
    && usermod --uid 1000 runner && groupmod --gid 1000 runner \
    && chown -R runner:runner /home/runner \
    && mkdir -p /home/runner/go/pkg /home/runner/.cache \
        /home/runner/.local/share/pnpm /home/runner/.local/share/vite-plus \
        /home/runner/.config/vite-plus /home/runner/.local/state/vite-plus \
    && chown -R runner:runner /home/runner/go /home/runner/.cache /home/runner/.local /home/runner/.config
COPY scripts/ci/install-lightpanda.sh /tmp/install-lightpanda.sh
RUN /tmp/install-lightpanda.sh /usr/local/bin && rm /tmp/install-lightpanda.sh
USER runner
ENV GOPATH=/home/runner/go GOCACHE=/home/runner/.cache/go-build
# mise-action uses the binary at this path rather than downloading one.
ARG MISE_VERSION=2026.10.11
ARG MISE_SHA256=f9612efcddda0ae55b62d3b6f82a6c860d5b54ebee0807d4f1a1d578055e1607
RUN mkdir -p /home/runner/.local/share/mise/bin \
    && curl -fsSL -o /home/runner/.local/share/mise/bin/mise \
        "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-x64" \
    && echo "${MISE_SHA256}  /home/runner/.local/share/mise/bin/mise" | sha256sum -c - \
    && chmod +x /home/runner/.local/share/mise/bin/mise
COPY --chown=runner:runner example/mise.toml /tmp/toolset/mise.toml
RUN cd /tmp/toolset \
    && MISE_TRUSTED_CONFIG_PATHS=/tmp/toolset MISE_CACHE_DIR=/tmp/mise-cache \
        /home/runner/.local/share/mise/bin/mise install \
    && rm -rf /tmp/toolset /tmp/mise-cache
WORKDIR /home/runner
ENTRYPOINT ["/home/runner/run.sh"]
