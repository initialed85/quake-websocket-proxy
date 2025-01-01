package ws

import (
	"bytes"
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

const sysTicRateHz = 20
const pingRate = time.Second * 1
const pingWriteTimeout = time.Second * 1
const readTimeout = time.Millisecond * (1000 / sysTicRateHz)
const writeTimeout = time.Millisecond * (1000 / sysTicRateHz)
const readTimeoutsPermitted = sysTicRateHz * 10
const writeTimeoutsPermitted = sysTicRateHz * 10

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
var portByUUID = make(map[string]int)

func getHandle(ctx context.Context, quakeServerAddr *net.UDPAddr) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastConnectionID++
		connectionID := lastConnectionID
		mu.Unlock()

		log := log.New(
			os.Stdout,
			fmt.Sprintf("%v\tWS\t", connectionID),
			log.Flags()|log.Lmsgprefix|log.Lmicroseconds,
		)

		log.Printf("REQ  <- %v", r.RemoteAddr)

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("error: failed upgrader.Upgrade for %v: %v", r.RemoteAddr, err)
			return
		}

		ports := make(chan int, 1)

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
					quakeServerAddr,
					serverToClient,
					clientToServer,
					connectionID,
					<-ports,
					func(name string, colour1, colour2 int) {
						// noop
					},
				)
				if err != nil {
					log.Printf("error: failed udp.RunClient for %v: %v", r.RemoteAddr, err)
				}
			}()
			runtime.Gosched()

			applyTimeouts := false

			go func() {
				defer cancel()

				readTimeouts := 0

				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					if applyTimeouts {
						wsConn.SetReadDeadline(time.Now().Add(readTimeout))
					}

					messageType, incomingMessage, err := wsConn.ReadMessage()
					if err != nil {
						if applyTimeouts {
							netErr, ok := err.(net.Error)
							if ok && netErr.Timeout() {
								readTimeouts++
								if readTimeouts < readTimeoutsPermitted {
									log.Printf("warning: failed conn.ReadMessage() for %v: %v", r.RemoteAddr, err)
									continue
								}
							}
						}

						log.Printf("error: failed conn.ReadMessage() for %v: %v", r.RemoteAddr, err)
						return
					}

					readTimeouts = 0

					switch messageType {
					case websocket.BinaryMessage:
						// log.Printf("RECV %v <- %v\t%v\t%#+v", "    ", r.RemoteAddr, len(incomingMessage), string(incomingMessage))

						if len(incomingMessage) > 6 {
							if bytes.Equal(incomingMessage[0:6], []byte{62, 91, 27, 3, 20, 0}) {
								uuid := string(incomingMessage[6:])

								mu.Lock()
								port, ok := portByUUID[uuid]
								if !ok {
									udpConn, _ := net.DialUDP("udp4", nil, quakeServerAddr)
									udpAddr := udpConn.LocalAddr().(*net.UDPAddr)
									port = udpAddr.Port
									_ = udpConn.Close()
									portByUUID[uuid] = port
								}
								mu.Unlock()

								ports <- port

								log.Printf("UUID %v -> %v\tport: %#+v, uuid: %#+v", "    ", quakeServerAddr.String(), port, uuid)
								continue
							}
						}

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

				t := time.NewTicker(pingRate)
				defer t.Stop()

				writeTimeouts := 0

				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						wsConn.SetWriteDeadline(time.Now().Add(pingWriteTimeout))
						err = wsConn.WriteMessage(websocket.PingMessage, []byte{})
						if err != nil {
							log.Printf("error: failed conn.WriteMessage() for [WebSocket ping] to %v: %v", r.RemoteAddr, err)
							return
						}
					case outgoingMessage := <-serverToClient:
						if applyTimeouts {
							wsConn.SetWriteDeadline(time.Now().Add(writeTimeout))
						}

						err = wsConn.WriteMessage(websocket.BinaryMessage, outgoingMessage)
						if err != nil {
							if applyTimeouts {
								netErr, ok := err.(net.Error)
								if ok && netErr.Timeout() {
									writeTimeouts++
									if writeTimeouts < writeTimeoutsPermitted {
										log.Printf("warning: failed conn.WriteMessage() for %#+v to %v: %v", outgoingMessage, r.RemoteAddr, err)
										continue
									}
								}
							}

							log.Printf("error: failed conn.WriteMessage() for %#+v to %v: %v", outgoingMessage, r.RemoteAddr, err)
							return
						}

						writeTimeouts = 0

						if !applyTimeouts {
							if len(outgoingMessage) == 9 {
								if bytes.Equal(outgoingMessage[0:2], []byte{0x80, 0x00}) {
									if outgoingMessage[4] == 0x81 {
										go func() {
											<-time.After(time.Second * 5)
											applyTimeouts = true
										}()
									}
								}
							}
						}

						// log.Printf("SEND %v -> %v\t%v\t%#+v", "    ", r.RemoteAddr, len(outgoingMessage), string(outgoingMessage))
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
	rawQuakeServerAddr := os.Getenv("QUAKE_SERVER_ADDRESS")
	quakeServerAddr, err := net.ResolveUDPAddr("udp4", rawQuakeServerAddr)
	if err != nil {
		return err
	}

	if quakeServerAddr.IP == nil || quakeServerAddr.Port <= 0 {
		return fmt.Errorf("%#+v parsed to invalid address %#+v", rawQuakeServerAddr, *quakeServerAddr)
	}

	http.HandleFunc("/ws", getHandle(ctx, quakeServerAddr))

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
