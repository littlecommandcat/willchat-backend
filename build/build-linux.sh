#!/usr/bin/env bash

APP_NAME="app"

echo "[Info] start building app"

export GOOS=linux
export GOARCH=amd64

go version
if [ $? -ne 0 ]; then
    echo "[Error] failed to find go. Please install Go or check PATH."
    exit 1
fi

if [ ! -d "../bin" ]; then
    mkdir -p "../bin"
    echo "[Info] create ../bin"
fi

go build -ldflags="-s -w" -o "../bin/${APP_NAME}_linux_amd64" ../cmd/server
if [ $? -ne 0 ]; then
    echo "[Error] failed to build"
    exit 1
fi

echo "[Success] output at ../bin/${APP_NAME}_linux_amd64"