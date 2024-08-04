package daemon

import (
	"encoding/json"
	"os"
	"path"

	wg "git.aetherial.dev/aeth/yosai/pkg/wireguard/centos"
)

type AddServerRequest struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Region string `json:"region"`
	Type   string `json:"type"`
}

type AddServerResponse struct {
	Id      int      `json:"id"`
	Ipv4    []string `json:"ipv4"`
	Label   string   `json:"label"`
	Created string   `json:"created"`
	Region  string   `json:"region"`
	Status  string   `json:"status"`
}

type StartWireguardRequest struct {
	InterfaceName string `json:"interface_name"`
}

type AddHostRequest struct {
	Target string `json:"target"`
}

type ConfigRenderRequest struct {
	Client       string `json:"client"`
	Server       string `json:"server"`
	OutputToFile bool   `json:"output_to_file"`
}

// Client for building internal Daemon route requests

func (c *Context) CreateServer(msg SockMessage) SockMessage {
	return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Server provisioned and indexed into Semaphore."))

}

/*
Route handler for all of the exposed functions that the daemon allows for

	:param msg: a SockMessage containing all of the request information
*/
func (c *Context) DaemonRouter(msg SockMessage) SockMessage {
	switch msg.Method {
	case "wg-up":
		var req StartWireguardRequest
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		out, err := wg.StartWgInterface(path.Join(c.Config.HostInfo.WireguardSavePath, req.InterfaceName+".conf"))
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, out)

	case "render-config":
		var req ConfigRenderRequest
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		serverKeypair, err := c.keyring.GetKey(req.Server + "_" + c.Keytags.WgServerKeypairKeyname())
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		clientKeypair, err := c.keyring.GetKey(req.Client + "_" + c.Keytags.WgClientKeypairKeyname())
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		server, err := c.Config.GetServer(req.Server)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		client, err := c.Config.GetClient(req.Client)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		seed := wg.WireguardTemplateSeed{
			VpnClientPrivateKey: clientKeypair.GetSecret(),
			VpnClientAddress:    client.VpnIpv4.String() + "/32",
			Peers: []wg.WireguardTemplatePeer{
				wg.WireguardTemplatePeer{
					Pubkey:  serverKeypair.GetPublic(),
					Address: server.WanIpv4,
					Port:    c.Config.VpnServerPort(),
				},
			}}
		cfg, err := wg.RenderClientConfiguration(seed)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		if req.OutputToFile == true {
			fpath := path.Join(c.Config.HostInfo.WireguardSavePath, server.Name+".conf")
			err = os.WriteFile(fpath, cfg, 0666)
			if err != nil {
				return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
			}
			return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Configuration saved to: "+fpath))
		}

		return *NewSockMessage(MsgResponse, REQUEST_OK, cfg)
	default:
		return *NewSockMessage(MsgResponse, REQUEST_UNRESOLVED, []byte("Unresolved Method"))
	}

}
