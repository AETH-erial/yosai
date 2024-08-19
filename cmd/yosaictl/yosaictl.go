package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
	case "daemon":
		switch args[1] {
		case "wg-up":
			resp, err := dClient.BringUpIntf(args[2])
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(string(resp.Body))
			os.Exit(0)
		case "init":
			err := dClient.ServiceInit(PRIMARY_SERVER)
			if err != nil {
				rb.Write([]byte(err.Error()))
			} else {
				rb.Write([]byte("Core system init success."))
			}
		case "rotate":
			cfg := dClient.GetConfig()
			switch len(cfg.Service.Servers) {
			case 0:
				// new server, start from new. Or reject and make the operator init
			case 1:
				/*
								standard rotation, flow would look something like:
								get server name ->
					            remove server from inventory ->
								start build for new server, different name ->
								wait for server to boot + configure itself ->
								## this is where firewall magic would happen that kills all traffic ##
								render new configuration for the new server ->
								bring down old wireguard interface ->
								bring up new wireguard interface ->
								run a health check and then a VPN/DNS leak test ->
								destroy old server from the system ->
								happy panda ->
				*/
				var oldServerName string
				var newServerName string
				var defaultClient string
				for name := range cfg.Service.Servers {
					oldServerName = name
				}
				for name := range cfg.Service.Clients {
					if cfg.Service.Clients[name].Default {
						defaultClient = cfg.Service.Clients[name].Name
					}
				}
				if defaultClient == "" {
					log.Fatal("No default client found. Please address this via the config file, and then run 'yosaictl config reload'")
				}
				switch oldServerName {
				case "":
					log.Fatal("couldnt capture the name of the old server: ", oldServerName)
				case PRIMARY_SERVER:
					newServerName = SECONDARY_SERVER
				case SECONDARY_SERVER:
					newServerName = PRIMARY_SERVER
				}
				if newServerName == "" {
					log.Fatal("couldnt capture the name of the new server:", newServerName)
				}
				err := dClient.RemoveServerFromAnsible(oldServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}

				err = dClient.NewServer(newServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				resp, err := dClient.PollServer(newServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				rb.Write(resp.Body)

				resp, err = dClient.ConfigureServers()
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				rb.Write(resp.Body)
				resp = dClient.Call([]byte(dclient.BLANK_JSON), "keyring", "reload")
				if resp.StatusCode != daemon.REQUEST_OK {
					rb.Write([]byte("Error reloading the keyring."))
					os.Exit(1)
				}
				resp = dClient.RenderWgConfig(fmt.Sprintf("server=%s,client=%s,outmode=save", newServerName, defaultClient))
				if resp.StatusCode != daemon.REQUEST_OK {
					rb.Write([]byte("Error rendering and saving the config."))
					os.Exit(1)
				}
				// firewall changes, when we get there
				err = dClient.LockFirewall()
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				// Bring down the interface here
				resp, err = dClient.BringDownIntf(oldServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				rb.Write(resp.Body)
				// Bring up the new interface here
				resp, err = dClient.BringUpIntf(newServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				rb.Write(resp.Body)
				// Run tests here
				resp, err = dClient.HealthCheck()
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				// destroy interface
				err = dClient.DestroyIntf(oldServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				// Destroy old server here
				err = dClient.DestroyServer(oldServerName)
				if err != nil {
					rb.Write([]byte(err.Error()))
					os.Exit(1)
				}
				// happy panda here

			default:
				rb.Write([]byte("Both primary and secondary VPN servers were found active. Manual intervention needed."))
				/*
					Handling this behaviour might be odd, we will need to have some utilities that allow an operator
					to save/manipulate their configuration/system. This could be that there are two servers that still exist,
					or maybe a dangling config, or maybe the values across the system are out of sync and need to be
					propogated across the system. The daemon configuration should have precedent where applicable.
					Non applicable examples would be if there is a discrepency between the cloud provider WAN IP, and the one
					on file.
				*/
			}

		case "render-wg":
			resp := dClient.RenderWgConfig(args[2])
			rb.Write(resp.Body)
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
