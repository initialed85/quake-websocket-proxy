package ws

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/initialed85/quake-websocket-proxy/pkg/udp"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: time.Second * 10,
	Subprotocols:     []string{"binary"},
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	EnableCompression: false,
}

var mu = new(sync.Mutex)
var lastConnectionID int64

func getHandle(ctx context.Context) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastConnectionID++
		connectionID := lastConnectionID
		mu.Unlock()

		log := log.New(os.Stdout, fmt.Sprintf("%v\tWS\t", connectionID), log.Flags()|log.Lmsgprefix)

		log.Printf("REQ  <- %v", r.RemoteAddr)

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("error: failed upgrader.Upgrade for %v: %v", r.RemoteAddr, err)
			return
		}

		log.Printf("UPG  <- %v", r.RemoteAddr)

		go func() {
			defer func() {
				_ = wsConn.WriteControl(
					websocket.CloseNormalClosure,
					[]byte(`{"reason": "Proxy goroutine shutting down"}`),
					time.Now().Add(time.Second*1),
				)
				_ = wsConn.Close()
			}()

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			serverToClient := make(chan []byte, 1024)
			clientToServer := make(chan []byte, 1024)
			defer close(clientToServer)

			go func() {
				defer cancel()

				err := udp.RunClient(
					ctx,
					cancel,
					os.Getenv("QUAKE_SERVER_ADDRESS"),
					serverToClient,
					clientToServer,
					connectionID,
				)
				if err != nil {
					log.Printf("error: failed udp.RunClient for %v: %v", r.RemoteAddr, err)
				}
			}()
			runtime.Gosched()

			go func() {
				defer cancel()

				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					messageType, incomingMessage, err := wsConn.ReadMessage()
					if err != nil {
						log.Printf("error: failed conn.ReadMessage() for %v: %v", r.RemoteAddr, err)
						return
					}

					switch messageType {
					case websocket.BinaryMessage:
						log.Printf("RECV %v <- %v\t%#+v", "    ", r.RemoteAddr, string(incomingMessage))

						select {
						case <-ctx.Done():
							return
						case clientToServer <- incomingMessage:
						}
					default:
						log.Printf("warning: unsupported message type %#+v for %#+v from %v", messageType, incomingMessage, r.RemoteAddr)
						continue
					}
				}
			}()
			runtime.Gosched()

			go func() {
				defer cancel()

				t := time.NewTicker(time.Second * 10)
				defer t.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						err = wsConn.WriteMessage(websocket.PingMessage, []byte{})
						if err != nil {
							log.Printf("error: failed conn.WriteMessage() for [WebSocket ping] to %v: %v", r.RemoteAddr, err)
							return
						}
					case outgoingMessage := <-serverToClient:
						err = wsConn.WriteMessage(websocket.BinaryMessage, outgoingMessage)
						if err != nil {
							log.Printf("error: failed conn.WriteMessage() for %#+v to %v: %v", outgoingMessage, r.RemoteAddr, err)
							return
						}

						log.Printf("SEND %v -> %v\t%#+v", "    ", r.RemoteAddr, string(outgoingMessage))
					}
				}
			}()
			runtime.Gosched()

			<-ctx.Done()
		}()
		runtime.Gosched()
	}
}

func RunServer(
	ctx context.Context,
	listenAddr string,
) error {
	http.HandleFunc("/ws", getHandle(ctx))

	localListenSrcAddr, err := net.ResolveTCPAddr("tcp4", listenAddr)
	if err != nil {
		return err
	}

	if localListenSrcAddr.IP == nil || localListenSrcAddr.Port <= 0 {
		return fmt.Errorf("%#+v parsed to invalid address %#+v", listenAddr, *localListenSrcAddr)
	}

	log.Printf("-1\tWS\tLSTN -> %v:%v", localListenSrcAddr.IP, localListenSrcAddr.Port)

	err = http.ListenAndServe(listenAddr, nil)
	if err != nil {
		return err
	}

	return nil
}
