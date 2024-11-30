# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

# Binary names
BINARY_NAME=EepTorrent
TOOLS_BINARY=EepTorrent-tools
ANACROLIX_BINARY=anacrolix-analyze
ANDROID_PACKAGE=com.i2p.eeptorrent

# Build directory
BUILD_DIR=bin
TOOLS_MAIN=cmd/tools/main.go
ANACROLIX_ANALYZE_MAIN=cmd/anacrolix/main.go

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
# Android and iOS parameters
ANDROID_APP_ID=com.i2p.eeptorrent
IOS_APP_ID=com.i2p.eeptorrent
ICON_PATH=images/Logo.png

# Targets
.PHONY: all build build-native build-linux build-windows build-macos build-android build-android-arm64 build-android-arm build-android-amd64 check-android clean test run install uninstall

all: test build

build: build-native build-linux build-windows build-macos build-android build-ios

build-native:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN)

build-tools:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(TOOLS_BINARY) -v $(TOOLS_MAIN)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(ANACROLIX_BINARY) -v $(ANACROLIX_ANALYZE_MAIN)

build-linux:
	@echo "Building for Linux..."
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 -v $(MAIN)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 -v $(MAIN)
	GOOS=linux GOARCH=arm $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm -v $(MAIN)

build-windows:
	@echo "Building for Windows..."
	mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe -v $(MAIN)
	GOOS=windows GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe -v $(MAIN)
	GOOS=windows GOARCH=386   $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-386.exe -v $(MAIN)

build-macos:
	@echo "Building for macOS..."
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-macos-amd64 -v $(MAIN)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-macos-arm64 -v $(MAIN)

build-ios:
	@echo "Building for iOS..."
	mkdir -p $(BUILD_DIR)
	fyne package -os ios \
		-icon $(ICON_PATH) \
		-appID $(IOS_APP_ID) \
		-name $(BINARY_NAME) \
		-appVersion $(VERSION)
	mv $(BINARY_NAME).ipa $(BUILD_DIR)/$(BINARY_NAME)-ios.ipa


build-android: build-android-arm64 build-android-arm build-android-amd64

build-android-arm64:
	mkdir -p $(BUILD_DIR)
	fyne package -os android/arm64 \
		-appID $(ANDROID_PACKAGE) \
		-name $(BINARY_NAME) \
		-icon $(ICON_PATH) \
		-appVersion $(VERSION) \
		-metadata MinSDK=21 \
		--exe $(BUILD_DIR)/$(BINARY_NAME)-arm64.apk
		mv EepTorrent.apk $(BUILD_DIR)/EepTorrent-arm64.apk

build-android-arm:
	mkdir -p $(BUILD_DIR)
	fyne package -os android/arm \
		-appID $(ANDROID_PACKAGE) \
		-name $(BINARY_NAME) \
		-icon $(ICON_PATH) \
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
		-icon $(ICON_PATH) \
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
	$(GOTEST) -v ./lib/...

run:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN)
	./$(BUILD_DIR)/$(BINARY_NAME)