package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	dclient "git.aetherial.dev/aeth/yosai/pkg/daemonclient"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
)

const PRIMARY_SERVER = "primary-vpn"
const SECONDARY_SERVER = "secondary-vpn"

func main() {
	var args []string
	args = os.Args[1:]
	dClient := dclient.DaemonClient{SockPath: dclient.UNIX_DOMAIN_SOCK_PATH}
	var rb = bytes.NewBuffer([]byte{})
	if strings.Contains(args[0], "ansible-") {
		req := semaphore.SemaphoreRequest{Target: args[2]}
		b, _ := json.Marshal(req)
		resp := dClient.Call(b, args[0], args[1])
		rb.Write(resp.Body)
	}
	switch args[0] {
	case "ansible":
		switch args[1] {
		case "bootstrap":
			err := dClient.BootstrapAll()
			if err != nil {
				rb.Write([]byte(err.Error()))
			}
			rb.Write([]byte("Ansible bootstrapped successfully."))
		}
	case "cloud":
		switch args[1] {
		case "delete":
			err := dClient.DestroyServer(args[2])
			if err != nil {
				rb.Write([]byte("Error deleting the server: " + args[2] + " Error: " + err.Error()))
			} else {
				rb.Write([]byte("Server: " + args[2] + " successfully removed."))
			}
		case "add":
			err := dClient.NewServer(args[2])
			if err != nil {
				rb.Write([]byte(err.Error()))
			}

		case "poll":
			resp, err := dClient.PollServer(args[2])
			if err != nil {
				rb.Write([]byte(err.Error()))
			}
			rb.Write(resp.Body)

		case "show":
			resp := dClient.Call([]byte(dclient.BLANK_JSON), "cloud", "show")
			rb.Write(resp.Body)
		}
	case "keyring":
		switch args[1] {
		case "show":
			if len(args) > 2 {
				b, _ := json.Marshal(daemon.KeyringRequest{Name: args[2]})
				resp := dClient.Call(b, "keyring", "show")
				rb.Write(resp.Body)
			} else {
				resp := dClient.Call([]byte(dclient.BLANK_JSON), "show", "all")
				rb.Write(resp.Body)

			}
		case "reload":
			resp := dClient.Call([]byte(dclient.BLANK_JSON), "keyring", "reload")
			rb.Write(resp.Body)

		}
	case "vpn-config":
		switch args[1] {
		case "save":
			resp := dClient.SaveWgConfig(args[2])
			rb.Write(resp.Body)
		case "show":
			resp := dClient.RenderWgConfig(args[2])
			rb.Write(resp.Body)
		}
	case "daemon":
		switch args[1] {
		case "show":
			resp := dClient.ShowAllRoutes()
			rb.Write(resp.Body)
		}
	case "config":
		switch args[1] {
		case "show":
			conf := dClient.GetConfig()
			b, _ := json.MarshalIndent(conf, " ", "    ")
			rb.Write(b)
		case "save":
			err := dClient.ForceSave()
			if err != nil {
				rb.Write([]byte(err.Error()))
			}
			rb.Write([]byte("Daemon configuration saved."))
		case "server":
			switch args[2] {
			case "add":
				err := dClient.AddServeToConfig(args[3])
				if err != nil {
					rb.Write([]byte(err.Error()))
				}
				rb.Write([]byte("Server added."))
			case "delete":
				b, _ := json.Marshal(daemon.VpnServer{Name: args[3]})
				resp := dClient.Call(b, "config-server", "delete")
				rb.Write(resp.Body)
			}
		case "client":
			switch args[2] {
			case "add":
				b, _ := json.Marshal(daemon.VpnClient{Name: args[3]})
				resp := dClient.Call(b, "config-peer", "delete")
				rb.Write(resp.Body)
			case "delete":
				b, _ := json.Marshal(daemon.VpnClient{Name: args[3]})
				resp := dClient.Call(b, "config-peer", "add")
				rb.Write(resp.Body)
			}
		case "reload":
			err := dClient.ForceReload()
			if err != nil {
				rb.Write([]byte(err.Error()))
			} else {
				rb.Write([]byte("configuration reloaded."))
			}

		}

	}
	out := bytes.NewBuffer([]byte{})
	err := json.Indent(out, rb.Bytes(), "", "    ")
	if err != nil {
		fmt.Println(string(rb.Bytes()))
		os.Exit(0)
	}
	fmt.Println(string(out.Bytes()))
}
