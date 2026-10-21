# Bob's ARM64 release runner executes under the host's QEMU binfmt handler.
# Go and the external-plugin witness both run as ARM64, including cgo.
FROM ghcr.io/actions/actions-runner:2.338.0
USER root
RUN apt-get update && apt-get install --yes build-essential pkg-config gh curl unzip \
    && rm -rf /var/lib/apt/lists/* \
    && userdel ubuntu \
    && usermod --uid 1000 runner && groupmod --gid 1000 runner \
    && chown -R runner:runner /home/runner \
    && mkdir -p /home/runner/go/pkg /home/runner/.cache \
    && chown -R runner:runner /home/runner/go /home/runner/.cache
USER runner
ENV GOPATH=/home/runner/go GOCACHE=/home/runner/.cache/go-build
WORKDIR /home/runner
ENTRYPOINT ["/home/runner/run.sh"]
