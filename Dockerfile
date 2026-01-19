FROM --platform=$BUILDPLATFORM golang:1.24.0 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ENV CGO_ENABLED=0

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/swarm-sentinel ./cmd/swarm-sentinel

# Distroless static image for minimal attack surface
# Contains only CA certificates and tzdata
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/swarm-sentinel /usr/local/bin/swarm-sentinel

# State directory - distroless nonroot user (65532) owns /home/nonroot
# Use /home/nonroot for writable state in distroless
ENV SS_STATE_PATH=/home/nonroot/state.json

USER nonroot:nonroot

EXPOSE 8080 9090

# Health check using the built-in endpoint
# Note: distroless has no shell, so we use the binary directly
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/swarm-sentinel", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/swarm-sentinel"]
