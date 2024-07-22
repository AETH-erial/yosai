package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func reader(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf[:])
		if err != nil {
			return
		}
		println("Client got:", string(buf[0:n]))
	}
}

func main() {

	if len(os.Args) < 4 {
		log.Fatal("Not enough arguments!")
	}
	var args []string
	args = os.Args[1:]

	msg := daemon.Marshal(daemon.SockMessage{
		Type:       daemon.MsgRequest,
		StatusMsg:  "",
		StatusCode: 0,
		Version:    daemon.SockMsgVers,
		Body:       []byte(args[2]),
		Target:     args[0],
		Method:     args[1],
	})

	conn, err := net.Dial("unix", UNIX_DOMAIN_SOCK_PATH)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	buf := bytes.NewBuffer(msg)
	_, err = io.Copy(conn, buf)
	if err != nil {
		log.Fatal("write error:", err)
	}
	resp := bytes.NewBuffer([]byte{})
	_, err = io.Copy(resp, conn)
	if err != nil {
		if err == io.EOF {
			fmt.Println("exited ok.")
			os.Exit(0)
		}
		log.Fatal(err)
	}
	var outbuf = bytes.NewBuffer([]byte{})
	fmt.Println(string(resp.Bytes()))

	json.Indent(outbuf, daemon.Unmarshal(resp.Bytes()).Body, " ", "    ")
	fmt.Println(string(outbuf.Bytes()))

}
