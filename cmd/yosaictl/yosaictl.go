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

const Key = "key"
const Cloud = "cloud"

func main() {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal(err)
	}
	// Keyring instantiation
	apikeyring := daemon.NewKeyRing()
	// Here we are demonstrating how you add a key to a keyring, in this
	// case it is the top level keyring.
	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, daemon.BearerAuth{
		Secret: os.Getenv(keytags.HASHICORP_VAULT_KEYNAME),
	})
	apikeyring.AddKey(keytags.LINODE_API_KEYNAME, daemon.BearerAuth{Secret: os.Getenv(keytags.LINODE_API_KEYNAME)})

	// creating the connection client with Hashicorp vault, and using the keyring we created above
	// as this clients keyring. This allows the API key we added earlier to be used when calling the API
	hashiConn := hashicorp.VaultConnection{
		VaultUrl:  os.Getenv("HASHICORP_VAULT_URL"),
		HttpProto: "http",
		KeyRing:   apikeyring,
		Client:    &http.Client{},
	}
	// Since the hashicorp.VaultConnection struct implements daemon.DaemonKeyRing,
	// we can add it to the apikeyring's 'Rungs' field.
	// When calling the apikeyring's .GetKey() method, it will check its
	// internal cache for that key, and then it will attempt to find that key in
	// each rung that it has on its keyring
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)

	if os.Args[1] == Key {
		method := os.Args[2]
		switch method {
		case "show":
			fmt.Println(apikeyring.GetKey(os.Args[3]))
			os.Exit(0)

		}
	}

	// testkey is the name of a key i created in my dev hashicorp vault, to show that you
	// can get a key from a child keyring via using the top level keyring.GetKey() method
	fmt.Println(apikeyring.GetKey("testkey"))
	// Grabbing a top level key from the parent keyring
	fmt.Println(apikeyring.GetKey(keytags.LINODE_API_KEYNAME))

}
