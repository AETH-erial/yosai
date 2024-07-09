package daemon

type DaemonActionOut struct {
	Content string
}

func (d DaemonActionOut) GetResult() string {
	return d.Content
}

func (c *Context) buildServerReq() (ActionIn, error) {
	var serverReq SockMessage
	var msg string
	serverReq = NewSockMessage("cloud", "new", "yosai-vpn-server")
	msg = Marshal(serverReq)
	return c.parseAction(msg)
}

func (c *Context) DaemonRouter(arg ActionIn) (ActionOut, error) {
	switch arg.Method() {
	case "bootstrap":

	}
	return DaemonActionOut{}, nil

}
