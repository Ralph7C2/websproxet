package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/urfave/cli/v2"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func main() {
	app := cli.NewApp()

	app.Commands = []*cli.Command{{
		Name:    "server",
		Aliases: []string{"s"},
		Usage:   "start a proxy server to listen for websocket connections and send to a tcp connection",
		Action: func(c *cli.Context) error {
			fmt.Println("bind:", c.String("bind"))
			http.ListenAndServe(c.String("bind"), serveWs(c.String("remote")))
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "bind",
				Usage: "the address to which to bind",
				Value: "0.0.0.0:8000",
			},
			&cli.StringFlag{
				Name:     "remote",
				Usage:    "the remote address to connect to",
				Required: true,
			},
		},
	}, {
		Name:    "proxy",
		Aliases: []string{"p"},
		Usage:   "start a proxy client",
		Action: func(c *cli.Context) error {
			serveTcp(c.String("bind"), c.String("remote"))
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "bind",
				Usage: "the local tcp address to listen on",
				Value: "127.0.0.1:8001",
			},
			&cli.StringFlag{
				Name:     "remote",
				Usage:    "the address of the remote websocket proxy server",
				Required: true,
			},
		},
	}}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func serveTcp(bind, remote string) {
	tcpListener, err := net.Listen("tcp", bind)
	if err != nil {
		log.Println(err)
		return
	}
	defer tcpListener.Close()

	for {
		tcpConn, err := tcpListener.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		fmt.Println("Got tcp conn")
		defer tcpConn.Close()
		wsConn, _, err := websocket.DefaultDialer.Dial(remote, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer wsConn.Close()

		fmt.Println("Proxying")
		proxy(tcpConn, wsConn)
		fmt.Println("done")
	}
}

func serveWs(remote string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer wsConn.Close()

		fmt.Println("Got ws conn")
		tcpConn, err := net.Dial("tcp", remote)
		if err != nil {
			log.Println(err)
			return
		}
		defer tcpConn.Close()
		fmt.Println("Proxying")
		proxy(tcpConn, wsConn)
		fmt.Println("Done")
	}
}

func proxy(tcpConn net.Conn, wsConn *websocket.Conn) {
	tcpWriter := bufio.NewWriter(tcpConn)
	tcpReader := bufio.NewReader(tcpConn)

	readDone := make(chan struct{})
	go func() {
		for {
			msgType, msg, err := wsConn.ReadMessage()
			if err != nil {
				log.Println(err)
				break
			}
			fmt.Println(msgType, string(msg))
			_, _ = tcpWriter.Write(msg)
			_ = tcpWriter.Flush()
		}
		close(readDone)
	}()
	writeDone := make(chan struct{})
	go func() {
		var buf [1024]byte
		for {
			n, err := tcpReader.Read(buf[:])
			if err != nil {
				log.Println(err)
				break
			}
			fmt.Println(string(buf[:n]))
			if err := wsConn.WriteMessage(2, buf[:n]); err != nil {
				log.Println(err)
				break
			}
		}
		close(writeDone)
	}()

	select {
	case <-readDone:
	case <-writeDone:
	}
}
