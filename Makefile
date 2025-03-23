all: server client

server:
	go build -o pomidoras-server pomidoras-server/main.go

client:
	go build -o bin/pomidorasctl pomidorasctl/main.go
