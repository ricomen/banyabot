# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH=amd64
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/instabot ./cmd/bot

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /data instabot
WORKDIR /app
COPY --from=build /out/instabot /app/instabot
COPY facts.json /app/facts.json
USER instabot
VOLUME ["/data"]
ENV DB_PATH=/data/banya.db \
    TZ=Asia/Almaty \
    FACTS_PATH=/app/facts.json
ENTRYPOINT ["/app/instabot"]
