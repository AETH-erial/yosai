package daemon

type Router interface {
	Routes() map[Method]func(SockMessage) SockMessage
	Register(Method, func(SockMessage) SockMessage)
}
