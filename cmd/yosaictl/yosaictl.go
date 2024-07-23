package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

/*
Build a JSON request to send the yosaid daemon

	    :param v: a struct to serialize for a request
		:param value: a string to put into the request
*/
func jsonBuilder(v interface{}, value string) []byte {
	addLn, ok := v.(linode.AddLinodeRequest)
	if ok {
		addLn = linode.AddLinodeRequest{
			Name: value,
		}
		b, _ := json.Marshal(addLn)
		return b

	}
	delLn, ok := v.(linode.DeleteLinodeRequest)
	if ok {
		delLn = linode.DeleteLinodeRequest{
			Id: value,
		}
		b, _ := json.Marshal(delLn)
		return b

	}
	semReq, ok := v.(semaphore.SemaphoreRequest)
	if ok {
		semReq = semaphore.SemaphoreRequest{
			Target: value,
		}
		b, _ := json.Marshal(semReq)
		return b

	}
	vaultReq, ok := v.(hashicorp.VaultItem)
	if ok {
		vals := strings.Split(value, ",")
		if len(vals) != 4 {
			log.Fatal("To add a key, you must pass the <name>,<type>,<public>,<private>. TODO: this interface needs to be improved.")
		}
		vaultReq = hashicorp.VaultItem{
			Name:   vals[0],
			Type:   vals[1],
			Public: vals[2],
			Secret: vals[3],
		}
		b, _ := json.Marshal(vaultReq)
		return b
	}
	return []byte("{\"data\":\"test\"}")

}

func main() {

	if len(os.Args) < 4 {
		log.Fatal("Not enough arguments!")
	}
	var args []string
	args = os.Args[1:]
	var rb = bytes.NewBuffer([]byte{})
	switch args[0] {
	case "cloud":
		switch args[1] {
		case "delete":
			rb.Write(jsonBuilder(linode.DeleteLinodeRequest{}, args[2]))
		case "add":
			rb.Write(jsonBuilder(linode.AddLinodeRequest{}, args[2]))
		}
	case "ansible-hosts":
		rb.Write(jsonBuilder(semaphore.SemaphoreRequest{}, args[2]))
	case "ansible-job":
		rb.Write(jsonBuilder(semaphore.SemaphoreRequest{}, args[2]))
	case "ansible-projects":
		rb.Write(jsonBuilder(semaphore.SemaphoreRequest{}, args[2]))
	case "ansible":
		rb.Write(jsonBuilder(semaphore.SemaphoreRequest{}, args[2]))
	case "keyring":
		rb.Write(jsonBuilder(hashicorp.VaultItem{}, fmt.Sprintf("%s,,,", args[2])))
	case "vault":
		rb.Write(jsonBuilder(hashicorp.VaultItem{}, args[2]))

	}

	msg := daemon.Marshal(daemon.SockMessage{
		Type:       daemon.MsgRequest,
		TypeLen:    int8(len(daemon.MsgRequest)),
		StatusMsg:  "",
		StatusCode: 0,
		Version:    daemon.SockMsgVers,
		Body:       rb.Bytes(),
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
