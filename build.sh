#! /usr/bin/bash


mkdir -p bin
rm -f bin/*
go build -o bin/19box-server ./cmd/server
go build -o bin/19box-admincli ./cmd/admincli
go build -o bin/19box-usercli ./cmd/usercli
go build -o bin/19box-auth ./cmd/auth
