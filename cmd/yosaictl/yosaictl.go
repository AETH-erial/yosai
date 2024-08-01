package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
)

func GetConfig() daemon.ConfigFromFile {

	request := daemon.SockMessage{
		Type:    daemon.MsgRequest,
		TypeLen: int8(len(daemon.MsgRequest)),
		Version: daemon.SockMsgVers,
		Body:    []byte("{\"data\":\"test\"}"),
		Target:  "config",
		Method:  "show",
	}
	resp := daemonRequest(request)
	var cfg daemon.ConfigFromFile
	err := json.Unmarshal(resp.Body, &cfg)
	if err != nil {
		log.Fatal("error unmarshalling config struct ", err.Error())
	}
	return cfg

}

func CreateServer(name string) daemon.SockMessage {
	b, _ := json.Marshal(linode.AddLinodeRequest{Name: name})
	request := daemon.SockMessage{
		Type:    daemon.MsgRequest,
		TypeLen: int8(len(daemon.MsgRequest)),
		Version: daemon.SockMsgVers,
		Body:    b,
		Target:  "cloud",
		Method:  "add",
	}
	return daemonRequest(request)

}

func AddServerToHosts(wan string, name string) daemon.SockMessage {
	b, _ := json.Marshal(daemon.VpnServer{WanIpv4: wan, Name: name})
	request := daemon.SockMessage{
		Type:    daemon.MsgRequest,
		TypeLen: int8(len(daemon.MsgRequest)),
		Version: daemon.SockMsgVers,
		Body:    b,
		Target:  "config",
		Method:  "add-server",
	}
	return daemonRequest(request)

}

func daemonRequest(msg daemon.SockMessage) daemon.SockMessage {
	conn, err := net.Dial("unix", UNIX_DOMAIN_SOCK_PATH)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	buf := bytes.NewBuffer(daemon.Marshal(msg))
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
	return daemon.Unmarshal(resp.Bytes())

}

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

/*
Build a JSON request to send the yosaid daemon

	    :param v: a struct to serialize for a request
		:param value: a string to put into the request
*/
func jsonBuilder(v interface{}, value string) []byte {
	addLn, ok := v.(linode.AddLinodeRequest)
	if ok {
		cfg := GetConfig()
		addLn = linode.AddLinodeRequest{
			Name:   value,
			Image:  cfg.Cloud.Image,
			Type:   cfg.Cloud.LinodeType,
			Region: cfg.Cloud.Region,
		}
		b, _ := json.Marshal(addLn)
		return b

	}
	delLn, ok := v.(linode.DeleteLinodeRequest)
	if ok {
		delLn = linode.DeleteLinodeRequest{
			Name: value,
		}
		b, _ := json.Marshal(delLn)
		return b

	}
	pollLn, ok := v.(linode.PollLinodeRequest)
	if ok {
		pollLn = linode.PollLinodeRequest{
			Address: value,
		}
		b, _ := json.Marshal(pollLn)
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
	renderConfigReq, ok := v.(daemon.ConfigRenderRequest)
	if ok {
		vals := strings.Split(value, ",")
		if len(vals) != 2 {
			log.Fatal("To render a config, you must pass the name of the server, followed by the client, i.e. yosai-vpn-server,iphone")
		}
		renderConfigReq = daemon.ConfigRenderRequest{
			Server: vals[0],
			Client: vals[1],
		}
		b, _ := json.Marshal(renderConfigReq)
		return b
	}
	return []byte("{\"data\":\"test\"}")

}

func main() {

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
		case "poll":
			rb.Write(jsonBuilder(linode.PollLinodeRequest{}, args[2]))
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
	case "daemon-add":
		rb.Write(jsonBuilder(linode.AddLinodeRequest{}, args[2]))
	case "daemon":
		rb.Write(jsonBuilder(daemon.ConfigRenderRequest{}, args[2]))
	case "config":
		switch args[1] {
		case "add-server":
			argSplit := strings.Split(args[2], ",")
			ipSplit := strings.Split(argSplit[0], ":")
			ip := ipSplit[0]
			port, err := strconv.Atoi(ipSplit[1])
			if err != nil {
				log.Fatal("Im not checking this thoroughly, but your port after the ':' should atleast be a valid number.")
			}
			b, _ := json.Marshal(daemon.VpnServer{WanIpv4: ip, Port: port, Name: argSplit[1]})
			rb.Write(b)
		case "add-peer":
			dataSp := strings.Split(args[2], ",")
			b, _ := json.Marshal(daemon.VpnClient{Name: dataSp[0], Pubkey: dataSp[1]})
			rb.Write(b)

		}

	}
	var msg daemon.SockMessage

	msg = daemon.SockMessage{
		Type:       daemon.MsgRequest,
		TypeLen:    int8(len(daemon.MsgRequest)),
		StatusMsg:  "",
		StatusCode: 0,
		Version:    daemon.SockMsgVers,
		Body:       rb.Bytes(),
		Target:     args[0],
		Method:     args[1],
	}

	var outbuf = bytes.NewBuffer([]byte{})
	responseMsg := daemonRequest(msg)

	err := json.Indent(outbuf, responseMsg.Body, " ", "    ")
	if err != nil {
		fmt.Println(string(responseMsg.Body))
		os.Exit(0)
	}
	fmt.Println(string(outbuf.Bytes()))

}
