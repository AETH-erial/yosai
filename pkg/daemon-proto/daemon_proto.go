package daemonproto

import (
	"bytes"
	"encoding/binary"
	"log"
)

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

/*
################################
### Protocol v2 status codes ###
################################
*/

const REQUEST_OK = 0
const REQUEST_TIMEOUT = 1
const REQUEST_FAILED = 2
const REQUEST_UNAUTHORIZED = 3
const REQUEST_ACCEPTED = 4
const REQUEST_UNRESOLVED = 5

/*
###############################
##### Protocol v2 methods #####
###############################
*/

type Method string

func MethodCheck(m string) (Method, error) {
	switch m {
	case "show":
		return SHOW, nil
	case "add":
		return ADD, nil
	case "delete":
		return DELETE, nil
	case "bootstrap":
		return BOOTSTRAP, nil
	case "reload":
		return RELOAD, nil
	case "poll":
		return POLL, nil
	case "run":
		return RUN, nil
	case "save":
		return SAVE, nil
	}
	return SHOW, &InvalidMethod{Method: m}

}

type InvalidMethod struct {
	Method string
}

func (i *InvalidMethod) Error() string {
	return "Invalid method passed: " + i.Method
}

const (
	SHOW      Method = "show"
	ADD       Method = "add"
	DELETE    Method = "delete"
	BOOTSTRAP Method = "bootstrap"
	RELOAD    Method = "reload"
	POLL      Method = "poll"
	RUN       Method = "run"
	SAVE      Method = "save"
)

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

func NewSockMessage(msgType string, statCode int8, body []byte) *SockMessage { // TODO: this function needs to be more versatile, and allow for additional more arguments
	return &SockMessage{Target: "",
		Method:     "",
		Body:       body,
		Version:    SockMsgVers,
		Type:       msgType,
		TypeLen:    int8(len(msgType)),
		StatusCode: statCode,
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
