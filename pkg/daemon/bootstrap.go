package daemon

import (
	"fmt"
)

type DaemonActionOut struct {
	Content string
}

func (d DaemonActionOut) GetResult() string {
	return d.Content
}

func (c *Context) buildServerReq(method string, arg string) (ActionIn, error) {
	var serverReq SockMessage
	var msg string
	serverReq = NewSockMessage("cloud", method, arg)
	msg = Marshal(serverReq)
	return c.parseAction(msg)
}

func (c *Context) buildSemaphoreReq(method string, arg string) (ActionIn, error) {
	var serverReq SockMessage
	var msg string
	serverReq = NewSockMessage("ansible", method, arg)
	msg = Marshal(serverReq)
	return c.parseAction(msg)
}

func (c *Context) DaemonRouter(arg ActionIn) (ActionOut, error) {
	var out DaemonActionOut
	ansibleTargets := []string{"keys", "inventory", "project"}
	switch arg.Method() {
	case "bootstrap":
		if arg.Arg() == "all" {
			var actions []ActionIn
			serverReq, err := c.buildServerReq("new", "yosai-vpn-server")
			if err != nil {
				return out, &InvalidAction{Msg: "Error building request for server request"}
			}
			actions = append(actions, serverReq)
			for i := range ansibleTargets {
				ansibleAction, err := c.buildSemaphoreReq("bootstrap", ansibleTargets[i])
				if err != nil {
					return out, &InvalidAction{Msg: "Error building request for: " + ansibleTargets[i]}
				}
				actions = append(actions, ansibleAction)
			}
			for i := range actions {
				_, err := c.resolveRoute(actions[i])
				if err != nil {
					return out, err
				}
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
	return out, nil

}
