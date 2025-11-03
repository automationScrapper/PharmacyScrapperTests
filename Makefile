APP_NAME=server
PKG=./cmd/server

.PHONY: all build run tidy clean

all: build

build:
	GOFLAGS=-trimpath CGO_ENABLED=0 go build -o $(APP_NAME) $(PKG)

run: build
	./$(APP_NAME)

tidy:
	go mod tidy

clean:
	rm -f $(APP_NAME)
