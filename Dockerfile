# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

# Build toolchains run natively on the BuildKit host. Without BUILDPLATFORM,
# the arm64 branch executes npm and the Go compiler through QEMU, which is much
# slower and makes npm ci appear to hang despite producing no progress output.
# ---- Stage 1: build the web frontend once on the native builder ----
FROM --platform=$BUILDPLATFORM node:24.15.0-alpine3.23@sha256:d1b3b4da11eefd5941e7f0b9cf17783fc99d9c6fc34884a665f40a06dbdfc94f AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Stage 2: cross-compile the Go binary on the native builder ----
FROM --platform=$BUILDPLATFORM golang:1.26.7-alpine3.23@sha256:b17af760035fc2f338eed92d448a6c67f2d45438844fc6c60678fa5f99e44b57 AS go-builder
RUN apk add --no-cache git
WORKDIR /src

ARG VERSION=0.1.0-dev
ARG BUILD_TIME=""
ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overlay the freshly built frontend so go:embed web/dist picks it up.
COPY --from=web-builder /web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags "-w -X vocat/internal/buildinfo.Version=${VERSION} -X vocat/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/vocat \
    ./cmd/vocat

# ---- Stage 3: minimal runtime ----
FROM alpine:3.23.3@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659
RUN apk add --no-cache ca-certificates ccid iproute2 pcsc-lite qmi-utils tzdata && \
    addgroup -S -g 1000 vocat && \
    adduser -S -D -H -u 1000 -G vocat vocat

RUN mkdir -p /opt/vocat/bin /opt/vocat/data && \
    chown -R vocat:vocat /opt/vocat

COPY --from=go-builder /out/vocat /opt/vocat/bin/vocat
COPY scripts/docker-entrypoint.sh /usr/local/bin/vocat-entrypoint

# Symlink into /usr/local/bin so `docker exec <ctr> vocat ...` finds it via $PATH.
RUN ln -s /opt/vocat/bin/vocat /usr/local/bin/vocat && \
    chmod 0755 /usr/local/bin/vocat-entrypoint

# This image is retained for local development only. Its entrypoint starts the
# bundled pcscd before VoCat; production runs the binary as the unprivileged
# vocat user under deploy/vocat.service in the dedicated VM.
USER root
VOLUME ["/opt/vocat/data"]
EXPOSE 7575
ENV VOCAT_ADDR=0.0.0.0:7575 \
    VOCAT_DATABASE_PATH=/opt/vocat/data/vocat.db

ENTRYPOINT ["/usr/local/bin/vocat-entrypoint"]
