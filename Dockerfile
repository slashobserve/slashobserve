FROM --platform=$BUILDPLATFORM oven/bun:1.4.2-slim@sha256:cb3bbbb08e13a4a2ff400f24c7a2a1d5efa83f6ef8544d52d95a519631e2fc61 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock web/bunfig.toml ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM --platform=$BUILDPLATFORM oven/bun:1.4.2-slim@sha256:cb3bbbb08e13a4a2ff400f24c7a2a1d5efa83f6ef8544d52d95a519631e2fc61 AS agent
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src/observe-agent
COPY observe-agent/package.json observe-agent/bun.lock observe-agent/bunfig.toml ./
RUN bun install --frozen-lockfile
COPY observe-agent/ ./
RUN case "$TARGETARCH" in amd64) bun_arch=x64 ;; arm64) bun_arch=arm64 ;; *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; esac && \
    bun build --compile --minify --target="bun-linux-${bun_arch}" --define "OBSERVE_AGENT_VERSION=\"${VERSION}\"" src/main.ts --outfile /out/observe-agent

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.23@sha256:146e3b9c7c1e2ec3d3f5348c8a686151a75e1976e50efac9fc022ebe446a9a82 AS server
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/server/webdist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/slashobserve-server ./cmd/slashobserve-server

FROM gcr.io/distroless/base-nossl-debian13:nonroot@sha256:6a2a3346335b94d3be0732472245f42ca3ce3cc1135a9521a2bda4b677f2ef0d
COPY --from=server /out/slashobserve-server /usr/local/bin/slashobserve-server
COPY --from=agent /out/observe-agent /usr/local/bin/observe-agent
ENV SLASHOBSERVE_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=12 \
    CMD ["/usr/local/bin/slashobserve-server", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/slashobserve-server"]
