# quake-websocket-proxy

This repo contains some Go code that can be used to expose a Quake 1 (NetQuake) server via WebSocket.

I played so many hours of Quake 1 as a kid and when I discovered [GMH-Code/Quake-WASM](https://github.com/GMH-Code/Quake-WASM) I was pretty excited.

The Emscripten magic didn't quite solve networking out of the box so I took the time to [make some additions for WebSocket multiplayer](https://github.com/initialed85/Quake-WASM) which required me to make some [small changes to the original Quake dedicated server](https://github.com/initialed85/Quake-LinuxUpdate) (shout out to [wyatt8740/Quake-LinuxUpdate](https://github.com/wyatt8740/Quake-LinuxUpdate) for the original work to get Quake building for Linux again).

Anyway, how can you use all this?

Well, if the Kubernetes cluster at my house is still working (doubtful), you can go to [quake.initialed85.cc](https://quake.initialed85.cc) and get stuck in.

Alternately, you can spin it all up locally using Docker Compose:

```shell
cd repos
git clone https://github.com/initialed85/Quake-WASM.git
git clone https://github.com/initialed85/Quake-LinuxUpdate.git
git clone https://github.com/initialed85/quake-websocket-proxy.git
cd quake-websocket-proxy
docker compose build
docker compose up
```

If that all works, then you can go to [http://localhost:8080](http://localhost:8080) and see it running.

To connect to the server, drop down the console and type `connect anything`; the server name can actually be anything, all paths lead to the WebSocket proxy.
