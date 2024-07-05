package main

import (
	"log"
	"net/http"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
	"github.com/joho/godotenv"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal(err)
	}
	apikeyring := daemon.NewKeyRing()
	// Here we are demonstrating how you add a key to a keyring, in this
	// case it is the top level keyring.
	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, daemon.BearerAuth{
		Secret: os.Getenv(keytags.HASHICORP_VAULT_KEYNAME),
	})
	apikeyring.AddKey(keytags.SEMAPHORE_API_KEYNAME, daemon.BearerAuth{Secret: os.Getenv(keytags.SEMAPHORE_API_KEYNAME)})

	// creating the connection client with Hashicorp vault, and using the keyring we created above
	// as this clients keyring. This allows the API key we added earlier to be used when calling the API
	hashiConn := hashicorp.VaultConnection{
		VaultUrl:  os.Getenv("HASHICORP_VAULT_URL"),
		HttpProto: "https",
		KeyRing:   apikeyring,
		Client:    &http.Client{},
	}
	lnConn := linode.LinodeConnection{Client: &http.Client{}, Keyring: apikeyring}
	semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
	apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)
	conf := daemon.ReadConfig("./.config.json")
	ctx := daemon.NewContext(UNIX_DOMAIN_SOCK_PATH, os.Stdout, apikeyring)
	ctx.Register("key", apikeyring.KeyringRouter)
	ctx.Register("config", conf.ConfigRouter)
	ctx.Register("cloud", lnConn.LinodeRouter)
	ctx.Register("ansible", semaphoreConn.SemaphoreRouter)
	ctx.ListenAndServe()
}
