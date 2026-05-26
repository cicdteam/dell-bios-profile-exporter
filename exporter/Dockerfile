# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/exporter ./cmd/dell-bios-profile-exporter

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends util-linux ca-certificates \
    && apt-get clean && rm -rf /var/lib/apt/lists/*
RUN useradd -u 65532 -r -s /sbin/nologin exporter
COPY --from=build /out/exporter /usr/local/bin/exporter
USER 65532
EXPOSE 9101
HEALTHCHECK --interval=30s --timeout=5s CMD ["/usr/local/bin/exporter", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/exporter"]
