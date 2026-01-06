#! /usr/bin/bash


if [ ! -d "bin" ]; then
	mkdir -p bin
else
    rm bin/19box-server 
    rm bin/19box-admincli 
    rm bin/19box-usercli 
    rm bin/19box-auth 
    rm bin/19box-webadmin 
fi

go build -o bin/19box-server ./cmd/server
go build -o bin/19box-admincli ./cmd/admincli
go build -o bin/19box-usercli ./cmd/usercli
go build -o bin/19box-auth ./cmd/auth
go build -o bin/19box-webadmin ./cmd/webadmin