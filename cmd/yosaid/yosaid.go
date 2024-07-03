package main

import (
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func main() {
	ctx := daemon.NewContext(UNIX_DOMAIN_SOCK_PATH, os.Stdout)
	ctx.ListenAndServe()
}
