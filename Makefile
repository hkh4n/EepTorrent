# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

# Binary names
BINARY_NAME=EepTorrent
ANDROID_PACKAGE=com.i2p.eeptorrent

# Build directory
BUILD_DIR=bin

# Main packages
MAIN=main.go

# Application version
VERSION=0.0.0

# Git commit hash (short)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# ldflags for injecting version and git commit
LDFLAGS=-ldflags "-X 'eeptorrent/lib/util.Version=$(VERSION)' -X 'eeptorrent/lib/util.GitCommit=$(GIT_COMMIT)'"

# Android parameters
ANDROID_SDK_ROOT?=$(ANDROID_HOME)
ANDROID_NDK_HOME?=$(ANDROID_NDK_ROOT)
MIN_SDK_VERSION=21


# Targets
.PHONY: all build build-linux-amd64 build-android build-android-arm64 build-android-arm build-android-amd64 check-android clean test run install uninstall

all: test build

build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN)

build-linux-amd64:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 -v $(MAIN)

build-android: build-android-arm64 build-android-arm

build-android-arm64:
	mkdir -p $(BUILD_DIR)
	fyne package -os android/arm64 \
		-appID $(ANDROID_PACKAGE) \
		-name $(BINARY_NAME) \
		-icon Icon.png \
		-appVersion $(VERSION) \
		-metadata MinSDK=21 \
		--exe $(BUILD_DIR)/$(BINARY_NAME)-arm64.apk
		mv EepTorrent.apk $(BUILD_DIR)/EepTorrent-arm64.apk

build-android-arm:
	mkdir -p $(BUILD_DIR)
	fyne package -os android/arm \
		-appID $(ANDROID_PACKAGE) \
		-name $(BINARY_NAME) \
		-icon Icon.png \
		-appVersion $(VERSION) \
		-metadata MinSDK=21 \
		--exe $(BUILD_DIR)/$(BINARY_NAME)-arm.apk
		mv EepTorrent.apk $(BUILD_DIR)/EepTorrent-arm.apk

build-android-amd64:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 \
	GOOS=android \
	GOARCH=amd64 \
	CC=$(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android21-clang \
	fyne package -os android/amd64 \
		-appID $(ANDROID_PACKAGE) \
		-name $(BINARY_NAME) \
		-icon Icon.png \
		-appVersion $(VERSION) \
		-metadata MinSDK=21 \
		--exe ./$(BUILD_DIR)/$(BINARY_NAME)-x86_64.apk
		mv EepTorrent.apk $(BUILD_DIR)/EepTorrent-amd64.apk

# Check android build environment
check-android:
	@echo "Checking Android build environment..."
	@if [ -z "$(ANDROID_SDK_ROOT)" ]; then \
		echo "Error: ANDROID_SDK_ROOT is not set"; \
		exit 1; \
	fi
	@if [ -z "$(ANDROID_NDK_HOME)" ]; then \
		echo "Error: ANDROID_NDK_HOME is not set"; \
		exit 1; \
	fi
	@echo "Android SDK: $(ANDROID_SDK_ROOT)"
	@echo "Android NDK: $(ANDROID_NDK_HOME)"

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

run:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN)
	./$(BUILD_DIR)/$(BINARY_NAME)