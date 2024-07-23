package daemon

import wg "git.aetherial.dev/aeth/yosai/pkg/wireguard/centos"

type DaemonActionOut struct {
	Content string
}

func (d DaemonActionOut) GetResult() string {
	return d.Content
}

func (c *Context) DaemonRouter(msg SockMessage) SockMessage {
	switch msg.Method {
	case "render-config":
		serverKeypair, err := c.keyring.GetKey("WG_SERVER_KEYPAIR")
		if err != nil {
			return *NewSockMessage(MsgResponse, []byte(err.Error()))
		}
		clientKeypair, err := c.keyring.GetKey("WG_CLIENT_KEYPAIR")
		if err != nil {
			return *NewSockMessage(MsgResponse, []byte(err.Error()))
		}
		seed := wg.WireguardTemplateSeed{
			VpnClientPrivateKey: clientKeypair.GetSecret(),
			VpnClientAddress:    c.Config.VpnClientIpAddr(),
			Peers: []wg.WireguardTemplatePeer{
				wg.WireguardTemplatePeer{
					Pubkey:  serverKeypair.GetPublic(),
					Address: c.Config.VpnClientIpAddr(),
					Port:    c.Config.VpnServerPort(),
				},
			}}
		cfg, err := wg.RenderClientConfiguration(c.VpnTempl, seed)
		return *NewSockMessage(MsgResponse, cfg)
	default:
		return *NewSockMessage(MsgResponse, []byte("Unresolved Method"))
	}
	//ansibleTargets := []string{"keys", "inventory", "project"}
	/*
		switch arg.Method() {
		case "bootstrap":
			if arg.Arg() == "all" {
				var requests []SockMessage
				serverReq, err := c.buildServerReq("new", "yosai-vpn-server")
				if err != nil {
					return out, &InvalidAction{Msg: "Error building request for server request"}
				}
				requests = append(requests, serverReq)
				for i := range ansibleTargets {
					ansibleAction, err := c.buildSemaphoreReq("bootstrap", ansibleTargets[i])
					if err != nil {
						return out, &InvalidAction{Msg: "Error building request for: " + ansibleTargets[i]}
					}
					requests = append(requests, ansibleAction)
				}
				for i := range requests {
					resp := c.resolveRoute(requests[i])
				}
				return DaemonActionOut{Content: "System bootstrapped."}, nil

			}
			return DaemonActionOut{}, nil
		case "reload":

			req, _ := c.parseAction(Marshal(NewSockMessage("cloud", "delete", fmt.Sprint(c.Config.VpnServerId()))))
			_, err := c.resolveRoute(req)
			if err != nil {
				return out, err
			}
			req, _ = c.parseAction(Marshal(NewSockMessage("ansible", "remove-hosts", c.Config.VpnServer())))
			_, err = c.resolveRoute(req)
			if err != nil {
				return out, err
			}
			req, _ = c.parseAction(Marshal(NewSockMessage("cloud", "new", fmt.Sprint(c.Config.ServerName()))))
			_, err = c.resolveRoute(req)
			if err != nil {
				return out, err
			}
			req, _ = c.parseAction(Marshal(NewSockMessage("ansible", "add-hosts", fmt.Sprint(c.Config.VpnServer()))))
			_, err = c.resolveRoute(req)
			if err != nil {
				return out, err
			}

			return DaemonActionOut{Content: "Upstream VPN server is being reloaded."}, nil

		}
	*/

}
