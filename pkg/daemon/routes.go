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
Render the wireguard configuration seed to be used when templating into the config file

	:param req: the struct containing the target client/server pair for the configuration
*/
func (c *Context) configSeed(req ConfigRenderRequest) (wg.WireguardTemplateSeed, error) {
	var seed wg.WireguardTemplateSeed
	serverKeypair, err := c.keyring.GetKey(req.Server + "_" + c.Keytags.WgKeypairKeyname())
	if err != nil {
		return seed, err
	}
	clientKeypair, err := c.keyring.GetKey(req.Client + "_" + c.Keytags.WgKeypairKeyname())
	if err != nil {
		return seed, err
	}
	server, err := c.Config.GetServer(req.Server)
	if err != nil {
		return seed, err
	}
	client, err := c.Config.GetClient(req.Client)
	if err != nil {
		return seed, err
	}
	seed = wg.WireguardTemplateSeed{
		VpnClientPrivateKey: clientKeypair.GetSecret(),
		VpnClientAddress:    client.VpnIpv4.String() + "/32",
		Peers: []wg.WireguardTemplatePeer{
			{
				Pubkey:  serverKeypair.GetPublic(),
				Address: server.WanIpv4,
				Port:    c.Config.VpnServerPort(),
			},
		}}
	return seed, nil
}

/*
wrapping the VPN show configuration function in a route friendly interface

	:param msg: a message to parse from the daemon socket
*/
func (c *Context) VpnShowHandler(msg SockMessage) SockMessage {
	var req ConfigRenderRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	seed, err := c.configSeed(req)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	cfg, err := wg.RenderClientConfiguration(seed)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	return *NewSockMessage(MsgResponse, REQUEST_OK, cfg)
}

/*
wrapping the VPN save configuration function in a route friendly interface

	:param msg: a message to parse from the daemon socket
*/
func (c *Context) VpnSaveHandler(msg SockMessage) SockMessage {
	var req ConfigRenderRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	seed, err := c.configSeed(req)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	cfg, err := wg.RenderClientConfiguration(seed)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	fpath := path.Join(c.Config.HostInfo.WireguardSavePath, req.Server+".conf")
	err = os.WriteFile(fpath, cfg, 0666)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Configuration saved to: "+fpath))
}

type VpnRouter struct {
	routes map[Method]func(SockMessage) SockMessage
}

func (v *VpnRouter) Register(method Method, callable func(SockMessage) SockMessage) {
	v.routes[method] = callable
}

func (v *VpnRouter) Routes() map[Method]func(SockMessage) SockMessage {
	return v.routes
}

func NewVpnRouter() *VpnRouter {
	return &VpnRouter{routes: map[Method]func(SockMessage) SockMessage{}}
}
