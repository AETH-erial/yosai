package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func main() {
	config := flag.Bool("config-init", false, "pass this to create a blank config at ./.config.tmpl")
	envInit := flag.Bool("env-init", false, "pass this to create a blank env file at ./.env.tmpl")
	flag.Parse()
	if *config {
		err := daemon.BlankConfig("./.config.tmpl")
		if err != nil {
			log.Fatal("Couldnt create blank config: ", err)

		}
		fmt.Println("Blank config created at ./.config.tmpl")
		os.Exit(0)
	}
	if *envInit {
		err := daemon.BlankEnv("./.env.tmpl")
		if err != nil {
			log.Fatal("Couldnt create blank env file: ", err)
		}
		fmt.Println("Blank env file created at ./.env.tmpl")
		os.Exit(0)
	}
	err := daemon.LoadAndVerifyEnv("./.env", daemon.EnvironmentVariables)
	if err != nil {
		log.Fatal("Error loading env file: ", err)
	}
	conf := daemon.ReadConfig("./.config.json")
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
	lnConn := linode.LinodeConnection{Client: &http.Client{}, Keyring: apikeyring, Config: conf}
	semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring, conf)
	apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)
	ctx := daemon.NewContext(UNIX_DOMAIN_SOCK_PATH, os.Stdout, apikeyring)
	ctx.Register("key", apikeyring.KeyringRouter)
	ctx.Register("config", conf.ConfigRouter)
	ctx.Register("cloud", lnConn.LinodeRouter)
	ctx.Register("ansible", semaphoreConn.SemaphoreRouter)
	ctx.ListenAndServe()
}
