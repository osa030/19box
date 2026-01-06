#! /usr/bin/bash

TOOLCHAIN_BIN="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin"
BIN_DIR="android"

if [ ! -d "$BIN_DIR" ]; then
	mkdir -p $BIN_DIR 
fi

export CC="$TOOLCHAIN_BIN/aarch64-linux-android34-clang"
echo "Using Android NDK CC=$CC"

GOOS=android GOARCH=arm64 CGO_ENABLED=1 go build -o $BIN_DIR/19box-server ./cmd/server
