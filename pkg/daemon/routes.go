package daemon

import (
	wg "git.aetherial.dev/aeth/yosai/pkg/wireguard/centos"
)

// Client for building internal Daemon route requests

/*
Route handler for all of the exposed functions that the daemon allows for

	:param msg: a SockMessage containing all of the request information
*/
func (c *Context) DaemonRouter(msg SockMessage) SockMessage {
	switch msg.Method {
	case "render-config":
		serverKeypair, err := c.keyring.GetKey(c.Keytags.WgServerKeypairKeyname())
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		clientKeypair, err := c.keyring.GetKey(c.Keytags.WgClientKeypairKeyname())
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		seed := wg.WireguardTemplateSeed{
			VpnClientPrivateKey: clientKeypair.GetSecret(),
			VpnClientAddress:    c.Config.VpnClientIpAddr(),
			Peers: []wg.WireguardTemplatePeer{
				wg.WireguardTemplatePeer{
					Pubkey:  serverKeypair.GetPublic(),
					Address: c.Config.VpnServer(),
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
