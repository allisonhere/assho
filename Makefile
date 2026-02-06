.PHONY: all build install uninstall run clean

BINARY_NAME=asshi
INSTALL_PATH=/usr/local/bin

all: build

build:
	go build -o $(BINARY_NAME) main.go

install: build
	sudo cp $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

uninstall:
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)

run:
	go run main.go

clean:
	go clean
	rm -f $(BINARY_NAME)