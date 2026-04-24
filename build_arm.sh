#! /usr/bin/bash

BIN_DIR="arm"

if [ ! -d "$BIN_DIR" ]; then
	mkdir -p $BIN_DIR 
fi


GOOS=linux GOARCH=arm64 go build -o $BIN_DIR/19box-server ./cmd/server
GOOS=linux GOARCH=arm64 go build -o $BIN_DIR/19box-webadmin ./cmd/webadmin
