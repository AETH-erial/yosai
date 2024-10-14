package daemon

import daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"

type Router interface {
	Routes() map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage
	Register(daemonproto.Method, func(daemonproto.SockMessage) daemonproto.SockMessage)
}
