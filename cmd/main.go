package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/initialed85/quake-websocket-proxy/pkg/ws"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := ws.RunServer(
		ctx,
		os.Getenv("WEBSOCKET_LISTEN_ADDRESS"),
	)
	if err != nil {
		log.Fatal(err)
	}

	for {
		time.Sleep(time.Second * 1)
	}
}
