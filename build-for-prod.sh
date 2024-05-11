#!/bin/bash

set -e

function teardown() {
    popd >/dev/null 2>&1 || true

    pushd "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1
    mv -fv ../Quake-WASM/WinQuake/net_websocket.c.bak ../Quake-WASM/WinQuake/net_websocket.c
    mv -fv ../Quake-WASM/WinQuake/Makefile.emscripten.bak ../Quake-WASM/WinQuake/Makefile.emscripten
    popd >/dev/null 2>&1 || true
}
trap teardown exit
pushd "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1

sed -i.bak s%ws://localhost:8081/ws%wss://quake-play.initialed85.cc/ws%g ../Quake-WASM/WinQuake/net_websocket.c
sed -i.bak s%ws://localhost:8081/ws%wss://quake-play.initialed85.cc/ws%g ../Quake-WASM/WinQuake/Makefile.emscripten

cd ../Quake-WASM
docker build --platform=linux/amd64 -t kube-registry:5000/quake-wasm:latest -f ./Dockerfile .

cd ../Quake-LinuxUpdate
docker build --platform=linux/amd64 -t kube-registry:5000/quake-server:latest -f ./Dockerfile .

cd ../quake-websocket-proxy
docker build --platform=linux/amd64 -t kube-registry:5000/quake-websocket-proxy:latest -f ./Dockerfile .

cd ./index
docker build --platform=linux/amd64 -t kube-registry:5000/quake-index:latest -f ./Dockerfile .

docker image push kube-registry:5000/quake-wasm:latest
docker image push kube-registry:5000/quake-server:latest
docker image push kube-registry:5000/quake-websocket-proxy:latest
docker image push kube-registry:5000/quake-index:latest
