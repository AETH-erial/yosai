package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"

	dclient "git.aetherial.dev/aeth/yosai/pkg/daemonclient"
)

const PRIMARY_SERVER = "primary-vpn"
const SECONDARY_SERVER = "secondary-vpn"

func printUsage() string {
	return "Insufficient arguments passed!\nUSAGE: yosaictl <TARGET> <METHOD> <ARGUMENTS>\nExample:\n    ~ yosaictl cloud show all"
}

func main() {
	var args []string
	args = os.Args[1:]
	dClient := dclient.DaemonClient{SockPath: dclient.UNIX_DOMAIN_SOCK_PATH}
	var rb = bytes.NewBuffer([]byte{})
	if len(args) < 2 {
		log.Fatal(printUsage())
	}
	target := args[0]
	method := args[1]
	var methodArgs []byte
	if len(args) == 3 {
		methodArgs = dclient.Pack(args[2])
	} else {
		methodArgs = []byte(dclient.BLANK_JSON)
	}
	resp := dClient.Call(methodArgs, target, method)
	rb.Write(resp.Body)
	out := bytes.NewBuffer([]byte{})
	err := json.Indent(out, rb.Bytes(), "", "    ")
	if err != nil {
		fmt.Println(string(rb.Bytes()))
		os.Exit(0)
	}
	fmt.Println(string(out.Bytes()))
}
