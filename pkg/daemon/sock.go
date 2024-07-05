package daemon

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const LogMsgTempl = "YOSAI Daemon ||| time: %s ||| %s\n"

const (
	Cloud     = "cloud"
	Ansible   = "ansible"
	Keys      = "key"
	Config    = "config"
	Daemon    = "daemon"
	Bootstrap = "bootstrap"
)

var Actions map[string]struct{} = map[string]struct{}{
	Cloud:     struct{}{},
	Ansible:   struct{}{},
	Keys:      struct{}{},
	Config:    struct{}{},
	Daemon:    struct{}{},
	Bootstrap: struct{}{},
}

type Action struct {
	target   string
	method   string
	callable func(args interface{}) (ActionOut, error)
	arg      string
}

/*
##########################################################
########### IMPLEMENTING THE ActionIn INTERFACE ##########
##########################################################
*/
func (a Action) Target() string { return a.target }
func (a Action) Method() string { return a.method }
func (a Action) Arg() string    { return a.arg }

type ActionIn interface {
	Target() string
	Method() string
	Arg() string
}

type ActionOut interface {
	GetResult() string
}

type Context struct {
	conn     net.Listener
	keyring  *ApiKeyRing
	routes   map[string]func(args ActionIn) (ActionOut, error)
	sockPath string
	rwBuffer bytes.Buffer
	stream   io.Writer
}

/*
Log a message to the Contexts 'stream' io.Writer interface
*/
func (c *Context) Log(data ...string) {
	c.stream.Write([]byte(fmt.Sprintf(LogMsgTempl, time.Now().String(), data)))

}

/*
Write a message back to the caller
*/
func (c *Context) Respond(conn net.Conn) {

	conn.Write(c.rwBuffer.Bytes())

}

func (c *Context) Handle(conn net.Conn) {
	defer conn.Close()
	b := make([]byte, 1024)
	nr, err := conn.Read(b)
	if err != nil {
		c.Log(err.Error())
		return
	}
	action, err := c.parseAction(b[0:nr])
	if err != nil {
		c.Log(err.Error())
		return
	}
	out, err := c.resolveRoute(action)
	if err != nil {
		c.Log("Error calling CLI action: ", err.Error())
	}
	_, err = conn.Write([]byte(out.GetResult()))
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
func NewContext(path string, rdr io.Writer, apiKeyring *ApiKeyRing) *Context {

	sock, err := net.Listen("unix", path)
	if err != nil {
		log.Fatal(err)
	}
	routes := map[string]func(args ActionIn) (ActionOut, error){}
	buf := make([]byte, 1024)
	return &Context{conn: sock, sockPath: path, rwBuffer: *bytes.NewBuffer(buf), stream: rdr, keyring: apiKeyring, routes: routes}

}

/*
Register a function to the daemons function router

	    :param name: the name to map the function to. This will dictate how the CLI will resolve a keyword to the target function
		:param callable: the callable that will be executed when the keyword from 'name' is passed to the CLI
*/
func (c *Context) Register(name string, callable func(args ActionIn) (ActionOut, error)) {
	c.routes[name] = callable
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
func (c *Context) parseAction(msg []byte) (ActionIn, error) {
	var action Action
	c.Log("Recieved request to parse action. ", string(msg))
	msgSplit := strings.Split(strings.Trim(string(msg), " "), " ")

	if len(msgSplit) < 3 {
		c.Log("Not enough arguments was passed to function call: ", fmt.Sprint(len(msgSplit)))
		return action, &InvalidAction{Action: "None", Msg: "Not Enough Args."}
	}

	_, ok := Actions[msgSplit[0]]
	if !ok {
		c.Log("Action not found: ", msgSplit[0])
		return action, &InvalidAction{Action: msgSplit[0], Msg: "Action not resolveable."}
	}
	target := msgSplit[0]
	method := msgSplit[1]
	arg := msgSplit[2]

	action = Action{
		target: target,
		method: method,
		arg:    arg,
	}
	return action, nil

}

/*
Resolve an action to a function
:param action: a parsed action from the sock stream
*/
func (c *Context) resolveRoute(action ActionIn) (ActionOut, error) {
	var out ActionOut
	handlerFunc, ok := c.routes[action.Target()]
	if !ok {
		return out, &InvalidAction{Msg: "Invalid Action", Action: action.Target()}
	}
	return handlerFunc(action)

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
