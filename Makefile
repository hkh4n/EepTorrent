# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

# Binary names
BINARY_NAME=EepTorrent

# Build directory
BUILD_DIR=bin

# Main packages
MAIN=main.go

# Targets
.PHONY: all build build-linux-amd64 clean test run install uninstall

all: test build

build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN)

build-linux-amd64:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 -v $(MAIN)


clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

run:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN)
	./$(BUILD_DIR)/$(BINARY_NAME)