FROM --platform=$BUILDPLATFORM golang:1.24.0 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/swarm-sentinel ./cmd/swarm-sentinel

FROM alpine:3.20

RUN addgroup -S sentinel && adduser -S -G sentinel -H -u 10001 sentinel \
    && apk add --no-cache ca-certificates \
    && mkdir -p /var/lib/swarm-sentinel \
    && chown -R sentinel:sentinel /var/lib/swarm-sentinel

COPY --from=builder /out/swarm-sentinel /usr/local/bin/swarm-sentinel

USER sentinel

EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/swarm-sentinel"]
