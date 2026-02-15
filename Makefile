.PHONY: all build install uninstall run clean

BINARY_NAME=assho
INSTALL_PATH=/usr/local/bin

all: build

build:
	go build -o $(BINARY_NAME) .

install: build
	sudo cp $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

uninstall:
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)

run:
	go run .

clean:
	go clean
	rm -f $(BINARY_NAME)
