# syntax=docker/dockerfile:1

# build: static, CGO-free binary cross-compiled to the target arch
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/sphragis ./cmd/sphragis
RUN mkdir -p /out/data

# runtime: distroless static, runs as nonroot (uid 65532); ca-certs ship in the base for TLS to providers and OTS calendars
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
LABEL org.opencontainers.image.title="sphragis" \
      org.opencontainers.image.description="EU AI Act compliance gateway: local PII redaction and a tamper-evident audit log" \
      org.opencontainers.image.url="https://sphragis.eu" \
      org.opencontainers.image.source="https://github.com/sphragis-oss/sphragis" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=build /out/sphragis /usr/local/bin/sphragis
# state dir (audit log, vault) owned by the runtime user; mount a volume here to persist
COPY --from=build --chown=65532:65532 /out/data /data
ENV SPHRAGIS_HOME=/data \
    SPHRAGIS_LISTEN_ADDR=":8787"
EXPOSE 8787
VOLUME ["/data"]
USER nonroot
ENTRYPOINT ["/usr/local/bin/sphragis"]
CMD ["serve"]
