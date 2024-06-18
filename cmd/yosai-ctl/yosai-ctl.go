package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	apikeyring := daemon.NewKeyRing()
	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, daemon.BearerAuth{Secret: os.Getenv(keytags.HASHICORP_VAULT_KEYNAME)})
	hashiConn := hashicorp.VaultConnection{
		VaultUrl:  os.Getenv("HASHICORP_VAULT_URL"),
		HttpProto: "http",
		KeyRing:   apikeyring,
		Client:    &http.Client{},
	}
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)

	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, daemon.BearerAuth{
		Secret: os.Getenv(keytags.HASHICORP_VAULT_KEYNAME),
	})
	apikeyring.AddKey(keytags.LINODE_API_KEYNAME, daemon.BearerAuth{Secret: os.Getenv(keytags.LINODE_API_KEYNAME)})
	fmt.Println(apikeyring.GetKey("testkey"))
	fmt.Println(apikeyring.GetKey(keytags.LINODE_API_KEYNAME))
}
