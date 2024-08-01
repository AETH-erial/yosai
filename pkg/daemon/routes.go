package daemon

import (
	"encoding/json"
	"fmt"

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

type AddHostRequest struct {
	Target string `json:"target"`
}

type ConfigRenderRequest struct {
	Client string `json:"client"`
	Server string `json:"server"`
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
		client, err := c.Config.GetClient(req.Client)
		seed := wg.WireguardTemplateSeed{
			VpnClientPrivateKey: clientKeypair.GetSecret(),
			VpnClientAddress:    client.VpnIpv4.String() + "/" + fmt.Sprint(c.Config.Service.VpnMask),
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
		return *NewSockMessage(MsgResponse, REQUEST_OK, cfg)
	default:
		return *NewSockMessage(MsgResponse, REQUEST_UNRESOLVED, []byte("Unresolved Method"))
	}

}
