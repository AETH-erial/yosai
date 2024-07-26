package daemon

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"text/template"
	"time"
)

// go:embed templates/*
var vpnTemplates embed.FS

const LogMsgTempl = "YOSAI Daemon ||| time: %s ||| %s\n"

/*
#########################################################
######## PROTOCOL RELATED FUNCTIONS AND STRUCTS #########
#########################################################
*/
const SockMsgVers = 2
const MESSAGE_RECIEVED = "MSG_RECV"
const UNRESOLVEABLE = "UNRESOLVEABLE"
const RequestOk = "OK"

/*
#################################
####### Protocol v2 area ########
#################################
*/
const VersionIdx = 0
const StatusCodeIdx = 1
const TypeInfoIdx = 2
const BodyInfoIdx = 3
const TargetInfoIdx = 11
const MethodInfoIdx = 19
const MsgHeaderEnd = 27

const MsgRequest = "REQUEST"
const MsgResponse = "RESPONSE"

type SockMessage struct {
	Type       string // the type of message being decoded
	TypeLen    int8   // The length of the Type, used for convenience when Marshalling
	StatusMsg  string // a string denoting what the output was, used in response messages
	StatusCode int8   // a status code that can be used to easily identify the type of error in response messages
	Version    int8   `json:"version"` // This is a version header for failing fast
	Body       []byte `json:"body"`    // The body of a SockMessage SHOULD be json decodable, to allow for complex data to get sent over
	Target     string `json:"target"`  // This target 'route' for where this message should be sent. Think of this like an HTTP URI/path
	Method     string `json:"method"`  // This is the method that we will be executing on the target endpoint. Think of this like the HTTP method
}

func NewSockMessage(msgType string, body []byte) *SockMessage { // TODO: this function needs to be more versatile, and allow for additional more arguments
	return &SockMessage{Target: "",
		Method:     "",
		Body:       body,
		Version:    SockMsgVers,
		Type:       msgType,
		TypeLen:    int8(len(msgType)),
		StatusCode: 0,
		StatusMsg:  RequestOk,
	}
}

/*
Takes in a SockMessage struct and serializes it so that it can be sent over a socket, and then decoded as a SockMessage

	:param v: a SockMessage to serialize for transportation
*/
func Marshal(v SockMessage) []byte { // TODO: Need to clean up the error handling here. This is really brittle. I just wanted to get it working
	msgHeader := []byte{}
	msgHeaderBuf := bytes.NewBuffer(msgHeader)
	preamble := []int8{
		SockMsgVers,
		v.StatusCode,
		v.TypeLen,
	}
	msgMeta := []int64{
		int64(len(v.Body)),
		int64(len(v.Target)),
		int64(len(v.Method)),
	}
	msgBody := [][]byte{
		[]byte(v.Type),
		v.Body,
		[]byte(v.Target),
		[]byte(v.Method),
	}
	for i := range preamble {
		err := binary.Write(msgHeaderBuf, binary.LittleEndian, preamble[i])
		if err != nil {
			log.Fatal("Fatal error when writing: ", preamble[i], " into message header buffer.")
		}
	}
	for i := range msgMeta {
		err := binary.Write(msgHeaderBuf, binary.LittleEndian, msgMeta[i])
		if err != nil {
			log.Fatal("Fatal error when writing: ", msgMeta[i], " into message header buffer.")
		}
	}
	for i := range msgBody {
		_, err := msgHeaderBuf.Write(msgBody[i])
		if err != nil {
			log.Fatal("Fatal error when writing: ", msgBody[i], " into message header buffer.")
		}
	}

	return msgHeaderBuf.Bytes()
}

/*
Unmarshalls a sock message byte array into a SockMessage struct, undoing what was done when Marshal() was called on the SockMessage

	:param msg: a byte array that can be unmarshalled into a SockMessage
*/
func Unmarshal(msg []byte) SockMessage {
	vers := int8(msg[VersionIdx])
	statusCode := int8(msg[StatusCodeIdx])
	typeInfo := int(msg[TypeInfoIdx])
	bodyInfo := int(binary.LittleEndian.Uint64(msg[BodyInfoIdx:TargetInfoIdx]))
	targetInfo := int(binary.LittleEndian.Uint64(msg[TargetInfoIdx:MethodInfoIdx]))
	msgPayload := msg[MsgHeaderEnd:]
	body := msgPayload[typeInfo : typeInfo+bodyInfo]
	var msgInfo = []string{
		string(msgPayload[0:typeInfo]),
		string(msgPayload[(typeInfo + bodyInfo):(typeInfo + bodyInfo + targetInfo)]),
		string(msgPayload[(typeInfo + bodyInfo + targetInfo):]),
	}
	return SockMessage{
		Type:       msgInfo[0],
		StatusCode: statusCode,
		StatusMsg:  MESSAGE_RECIEVED,
		Version:    vers,
		Body:       body,
		Target:     msgInfo[1],
		Method:     msgInfo[2],
	}
}

/*
##########################################################
########### IMPLEMENTING THE ActionIn INTERFACE ##########
##########################################################
*/

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
	routes   map[string]func(req SockMessage) SockMessage
	sockPath string
	Config   Configuration
	rwBuffer bytes.Buffer
	stream   io.Writer
	VpnTempl *template.Template
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
	b := make([]byte, 2048)
	nr, err := conn.Read(b)
	if err != nil {
		c.Log(err.Error())
		return
	}
	req := c.parseRequest(b[0:nr])
	out := c.resolveRoute(req)
	_, err = conn.Write(Marshal(out))
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
func NewContext(path string, rdr io.Writer, apiKeyring *ApiKeyRing, conf Configuration) *Context {

	sock, err := net.Listen("unix", path)
	if err != nil {
		log.Fatal(err)
	}
	vpnTmpl, err := template.ParseFS(vpnTemplates, "templates/*")
	if err != nil {
		log.Fatal(err)
	}
	routes := map[string]func(req SockMessage) SockMessage{}
	buf := make([]byte, 1024)
	return &Context{conn: sock, sockPath: path, rwBuffer: *bytes.NewBuffer(buf), stream: rdr, keyring: apiKeyring,
		routes: routes, Config: conf, VpnTempl: vpnTmpl}

}

/*
Register a function to the daemons function router

	    :param name: the name to map the function to. This will dictate how the CLI will resolve a keyword to the target function
		:param callable: the callable that will be executed when the keyword from 'name' is passed to the CLI
*/
func (c *Context) Register(name string, callable func(req SockMessage) SockMessage) {
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
func (c *Context) parseRequest(msg []byte) SockMessage {
	c.Log("Recieved request to parse action. ", string(msg))

	return Unmarshal(msg)

}

/*
Resolve an action to a function
:param action: a parsed action from the sock stream
*/
func (c *Context) resolveRoute(req SockMessage) SockMessage {
	handlerFunc, ok := c.routes[req.Target]
	if !ok {
		err := &InvalidAction{Msg: "Invalid Action", Action: req.Target}
		return SockMessage{StatusMsg: UNRESOLVEABLE, Body: []byte(err.Error())}
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
