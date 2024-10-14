package daemon

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/keyring"
)

type Context struct {
	conn     net.Listener
	keyring  *keyring.ApiKeyRing
	Keytags  keytags.Keytagger
	routes   map[string]Router
	sockPath string
	Config   *config.Configuration
	servers  []config.VpnServer
	rwBuffer bytes.Buffer
	stream   io.Writer
}

/*
Show all of the route information for the context

	:param msg: a message to parse from the daemon socket
*/
func (c *Context) ShowRoutesHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var data string
	for k, v := range c.routes {
		data = data + k + "\n"

		routes := v.Routes()
		for i := range routes {
			data = data + "\u0009" + string(i) + "\n"

		}
		data = data + "\n"

	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte(data))

}

/*
Context router
*/
type ContextRouter struct {
	routes map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage
}

func (c *ContextRouter) Register(method daemonproto.Method, callable func(daemonproto.SockMessage) daemonproto.SockMessage) {
	c.routes[method] = callable
}

func (c *ContextRouter) Routes() map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage {
	return c.routes
}

func NewContextRouter() *ContextRouter {
	return &ContextRouter{routes: map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage{}}
}

/*
Write a message back to the caller
*/
func (c *Context) Respond(conn net.Conn) {

	conn.Write(c.rwBuffer.Bytes())

}

/*
Log wrapper for the context struct

	:param data: string arguments to send to the logger
*/
func (c *Context) Log(msg ...string) {
	dMsg := []string{"daemon.Context:"}
	dMsg = append(dMsg, msg...)
	c.Config.Log(dMsg...)

}

func (c *Context) Handle(conn net.Conn) {
	defer conn.Close()
	b := make([]byte, 2048)
	nr, err := conn.Read(b)
	if err != nil {
		c.Log(err.Error())
		return
	}
	req := c.parseRequest(b[0:nr])
	out := c.resolveRoute(req)
	_, err = conn.Write(daemonproto.Marshal(out))
	if err != nil {
		c.Log(err.Error())
		return
	}

}

/*
spawns subroutines to listen for different syscalls
*/
func (c *Context) handleSyscalls() {

	// Cleanup the sockfile.
	chanSig := make(chan os.Signal, 1)
	signal.Notify(chanSig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-chanSig
		os.Remove(c.sockPath)
		os.Exit(1)
	}()
}

/*
Open a daemon context pointer
*/
func NewContext(path string, rdr io.Writer, apiKeyring *keyring.ApiKeyRing, conf *config.Configuration) *Context {

	sock, err := net.Listen("unix", path)
	if err != nil {
		log.Fatal(err)
	}
	routes := map[string]Router{}
	buf := make([]byte, 1024)
	return &Context{conn: sock, sockPath: path, rwBuffer: *bytes.NewBuffer(buf), stream: rdr, keyring: apiKeyring,
		routes: routes, Config: conf, Keytags: keytags.ConstKeytag{}}

}

/*
Register a function to the daemons function router

	    :param name: the name to map the function to. This will dictate how the CLI will resolve a keyword to the target function
		:param callable: the callable that will be executed when the keyword from 'name' is passed to the CLI
*/
func (c *Context) Register(name string, router Router) {
	c.routes[name] = router
}

/*
Hold the execution context open and listen for input
*/
func (c *Context) ListenAndServe() {
	c.handleSyscalls()
	for {
		conn, err := c.conn.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go c.Handle(conn)

	}
}

/*
Validate and parse a stream from the unix socket and return an Action

	:param msg: a byte array with the action and arguments
*/
func (c *Context) parseRequest(msg []byte) daemonproto.SockMessage {
	c.Log("Recieved request to parse action. ", string(msg))

	return daemonproto.Unmarshal(msg)

}

/*
Resolve an action to a function
:param action: a parsed action from the sock stream
*/
func (c *Context) resolveRoute(req daemonproto.SockMessage) daemonproto.SockMessage {
	router, ok := c.routes[req.Target]
	if !ok {
		err := InvalidAction{Msg: "Invalid Action", Action: req.Target}
		c.Log("Error finding a router for target: ", req.Target)
		return daemonproto.SockMessage{StatusMsg: daemonproto.UNRESOLVEABLE, Body: []byte(err.Error())}
	}
	method, err := daemonproto.MethodCheck(req.Method)
	if err != nil {
		c.Log("Error parsing request: ", string(daemonproto.Marshal(req)), err.Error())
	}
	handlerFunc, ok := router.Routes()[method]
	if !ok {
		err := InvalidAction{Msg: "Unimplemented method", Action: req.Method}
		c.Log("Error invoking the method: ", req.Method, "on the target: ", req.Target)
		return daemonproto.SockMessage{StatusMsg: daemonproto.UNRESOLVEABLE, Body: []byte(err.Error())}
	}

	return handlerFunc(req)

}

/*
###########################################
################ ERRORS ###################
###########################################
*/

type InvalidAction struct {
	Msg    string
	Action string
}

func (i *InvalidAction) Error() string {
	return fmt.Sprintf("Invalid action: '%s' parsed. Error: %s", i.Action, i.Msg)
}

type DaemonIoError struct {
	Msg    []byte
	Action string
}

func (d *DaemonIoError) Error() string {
	return fmt.Sprintf("There was an error %s. Message: %s", d.Action, string(d.Msg))
}
