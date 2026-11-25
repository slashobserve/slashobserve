FROM golang:1.27.4-alpine3.23@sha256:146e3b9c7c1e2ec3d3f5348c8a686151a75e1976e50efac9fc022ebe446a9a82 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY tools/codeserver ./tools/codeserver
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/codeserver ./tools/codeserver

FROM gcr.io/distroless/base-nossl-debian13:nonroot@sha256:89815d620ac01d8de13cdaf04d19ed874447ca2296d4985b14b818f6e5e47854
COPY --from=build /out/codeserver /usr/local/bin/codeserver
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=12 \
    CMD ["/usr/local/bin/codeserver", "-healthcheck", "-addr", ":8080"]
ENTRYPOINT ["/usr/local/bin/codeserver"]
CMD ["-root", "/code", "-addr", ":8080"]
