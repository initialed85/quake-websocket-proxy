FROM golang:1.21

WORKDIR /srv/

COPY go.mod /srv/go.mod
COPY go.sum /srv/go.sum
RUN go get ./...

COPY cmd /srv/cmd
COPY pkg /srv/pkg

RUN go build -o quake-websocket-proxy ./cmd

ENV WEBSOCKET_LISTEN_ADDRESS=0.0.0.0:8081
ENV QUAKE_SERVER_ADDRESS=quake-server:26000

EXPOSE 8081

ENTRYPOINT ["./quake-websocket-proxy"]
CMD []
