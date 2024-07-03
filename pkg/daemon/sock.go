package daemon

import (
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

type Context struct {
	conn     net.Listener
	sockPath string
	rwBuffer []byte
	stream   io.Writer
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
func NewContext(path string, rdr io.Writer) *Context {

	sock, err := net.Listen("unix", path)
	if err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, 1024)
	return &Context{conn: sock, sockPath: path, rwBuffer: buf, stream: rdr}

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
		go func(con net.Conn) {
			defer conn.Close()
			_, err := conn.Read(c.rwBuffer)
			if err != nil {
				c.stream.Write([]byte(err.Error()))
				return
			}
			c.stream.Write(c.rwBuffer)
			_, err = conn.Write(c.rwBuffer)
			if err != nil {
				log.Fatal("Write: ", err)
			}
		}(conn)

	}
}
