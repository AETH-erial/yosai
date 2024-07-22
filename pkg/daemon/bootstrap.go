package daemon

type DaemonActionOut struct {
	Content string
}

func (d DaemonActionOut) GetResult() string {
	return d.Content
}

func (c *Context) DaemonRouter(arg ActionIn) (ActionOut, error) {
	var out DaemonActionOut
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
	return out, nil

}
