.PHONY: test build build-linux-amd64 build-linux-arm64 build-linux-armv7 docker docker-arm64

MODULE=./cmd/bot
BIN=bin/instabot

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) $(MODULE)

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN)-linux-amd64 $(MODULE)

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(BIN)-linux-arm64 $(MODULE)

build-linux-armv7:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o $(BIN)-linux-armv7 $(MODULE)

docker:
	docker compose build

docker-arm64:
	docker buildx build --platform linux/arm64 -t instabot:arm64 --load .
