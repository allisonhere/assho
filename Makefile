.PHONY: all build install uninstall run clean

BINARY_NAME=assho
INSTALL_PATH=/usr/local/bin
MAN_PATH=/usr/local/share/man/man1
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
LDFLAGS=-s -w -X main.version=$(VERSION)

all: build

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .

install: build
	sudo cp $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	sudo install -d $(MAN_PATH)
	sudo install -m 0644 $(BINARY_NAME).1 $(MAN_PATH)/$(BINARY_NAME).1

uninstall:
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	sudo rm -f $(MAN_PATH)/$(BINARY_NAME).1

run:
	go run .

clean:
	go clean
	rm -f $(BINARY_NAME)
