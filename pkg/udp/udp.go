package udp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

func RunClient(
	ctx context.Context,
	cancel context.CancelFunc,
	rawDstAddr string,
	serverToClient chan []byte,
	clientToServer chan []byte,
	connectionID int64,
) error {
	defer cancel()

	log := log.New(os.Stdout, fmt.Sprintf("%v\tUDP\t", connectionID), log.Flags()|log.Lmsgprefix)

	dstAddr := &net.UDPAddr{}
	mode := "CTRL"

	defer func() {
		log.Printf("DONE %v -> %v:%v", mode, dstAddr.IP.String(), dstAddr.Port)
	}()

	originalDstAddr, err := net.ResolveUDPAddr("udp4", rawDstAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to control server: %v", err)
	}

	if originalDstAddr.IP == nil || originalDstAddr.Port <= 0 {
		err = fmt.Errorf("%#+v parsed to invalid address %#+v", rawDstAddr, *originalDstAddr)
		return fmt.Errorf("failed to connect to control server: %v", err)
	}

	dstAddr.IP = originalDstAddr.IP
	dstAddr.Port = originalDstAddr.Port
	dstAddr.Zone = originalDstAddr.Zone

	log.Printf("CONN %v -> %v:%v", mode, dstAddr.IP, dstAddr.Port)

	udpConn, _ := net.DialUDP("udp4", nil, dstAddr)
	localControlSrcAddr := udpConn.LocalAddr().(*net.UDPAddr)
	_ = udpConn.Close()
	localControlSrcAddr.Port = 0

	udpConn, err = net.ListenUDP("udp4", localControlSrcAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to control server: %v", err)
	}

	defer func() {
		_ = udpConn.Close()
	}()

	go func() {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			b := make([]byte, 65536)
			udpConn.SetReadDeadline(time.Now().Add(time.Second * 10))
			n, remoteSrcAddr, err := udpConn.ReadFrom(b)
			if err != nil {
				log.Printf("error: failed udpConn.ReadFrom() for %v: %v", dstAddr.String(), err)
				return
			}
			incomingMessage := b[:n]

			log.Printf("RECV %v <- %v\t%#+v (%#+v)", mode, remoteSrcAddr.String(), incomingMessage, string(incomingMessage))

			if mode == "CTRL" {
				if len(incomingMessage) == 9 {
					if bytes.Equal(incomingMessage[0:2], []byte{0x80, 0x00}) {
						if incomingMessage[4] == 0x81 {
							dstAddr.Port = int(binary.LittleEndian.Uint16(incomingMessage[5:7]))
							mode = "GAME"
						}
					}
				}
			}

			select {
			case <-time.After(time.Second * 10):
				return
			case serverToClient <- incomingMessage:
			}
		}
	}()

	go func() {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 10):
				return
			case outgoingMessage := <-clientToServer:
				// if bytes.HasPrefix(outgoingMessage, []byte{0xff, 0xff, 0xff, 0xff}) {
				// 	log.Printf("DROP %v -> %v\t%#+v (%#+v)", mode, dstAddr.String(), outgoingMessage, string(outgoingMessage))
				// 	continue
				// }

				func() {
					log.Printf("SEND %v -> %v\t%#+v (%#+v)", mode, dstAddr.String(), outgoingMessage, string(outgoingMessage))

					udpConn.SetWriteDeadline(time.Now().Add(time.Second * 10))
					_, err = udpConn.WriteToUDP(outgoingMessage, dstAddr)
					if err != nil {
						log.Printf("error: failed udpConn.Write() for %#+v to %v: %v", outgoingMessage, dstAddr.String(), err)
						cancel()
						return
					}
				}()
			}
		}
	}()

	<-ctx.Done()

	return nil
}
