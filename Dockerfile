# syntax=docker/dockerfile:1.7

# Build stage: compile a static binary for the target platform using Go 1.26.
FROM --platform=$BUILDPLATFORM golang:1.26.1-bookworm AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ejinagrid ./cmd/ejinagrid

# Runtime stage: minimal image containing only the compiled service.
FROM scratch
COPY --from=builder /out/ejinagrid /ejinagrid
ENV EJINA_ADDR=:51523 \
    EJINA_DATA=/data/ejinagrid.json \
    EJINA_INSPECTION_WINDOW_START=8 \
    EJINA_INSPECTION_WINDOW_END=10 \
    EJINA_TICK_SECONDS=30
EXPOSE 51523
ENTRYPOINT ["/ejinagrid"]
