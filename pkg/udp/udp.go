package udp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"time"
)

func RunClient(
	ctx context.Context,
	cancel context.CancelFunc,
	originalDstAddr *net.UDPAddr,
	serverToClient chan []byte,
	clientToServer chan []byte,
	connectionID int64,
	listenPort int,
	handlePlayerInformation func(name string, colour1 int, colour2 int),
) error {
	defer cancel()
	defer close(serverToClient)

	log := log.New(
		os.Stdout,
		fmt.Sprintf("%v\tUDP\t", connectionID),
		log.Flags()|log.Lmsgprefix|log.Lmicroseconds,
	)

	dstAddr := &net.UDPAddr{}
	mode := "CTRL"
	defer func() {
		log.Printf("DONE %v -> %v:%v", mode, dstAddr.IP.String(), dstAddr.Port)
	}()

	dstAddr.IP = originalDstAddr.IP
	dstAddr.Port = originalDstAddr.Port
	dstAddr.Zone = originalDstAddr.Zone

	log.Printf("CONN %v -> %v:%v", mode, dstAddr.IP, dstAddr.Port)

	listenAddr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%v", listenPort))
	udpConn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to control server: %v", err)
	}

	defer func() {
		_, _ = udpConn.WriteToUDP([]byte{0x02}, dstAddr)
		udpConn.SetReadDeadline(time.Now().Add(time.Second * 1))
		_, _, _ = udpConn.ReadFrom(make([]byte, 65536))
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
			n, remoteSrcAddr, err := udpConn.ReadFrom(b)
			if err != nil {
				log.Printf("error: failed udpConn.ReadFrom() for %v: %v", dstAddr.String(), err)
				return
			}
			incomingMessage := b[:n]

			_ = remoteSrcAddr
			log.Printf("RECV %v <- %v\t%v\t%#+v", mode, remoteSrcAddr.String(), len(incomingMessage), string(incomingMessage))

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
			case <-ctx.Done():
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
			case outgoingMessage := <-clientToServer:
				func() {
					log.Printf("SEND %v -> %v\t%v\t%#+v", mode, dstAddr.String(), len(outgoingMessage), string(outgoingMessage))

					if bytes.Contains(outgoingMessage, []byte{0x04, 'n', 'a', 'm', 'e'}) &&
						bytes.Contains(outgoingMessage, []byte{0x04, 'c', 'o', 'l', 'o', 'r'}) {
						s := string(outgoingMessage)

						name := regexp.MustCompile("\x04name \"(.*)\"\n").FindStringSubmatch(s)[1]
						rawColor := regexp.MustCompile("\x04color (\\d+) (\\d+)\n").FindStringSubmatch(s)
						colour1, _ := strconv.ParseInt(rawColor[1], 10, 64)
						colour2, _ := strconv.ParseInt(rawColor[2], 10, 64)

						log.Printf("JOIN %v -> %v\tname: %v, colour1: %v, colour2: %v", mode, dstAddr.String(), name, colour1, colour2)

						handlePlayerInformation(name, int(colour1), int(colour2))
					}

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
