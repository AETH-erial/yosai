package main

import (
	"fmt"
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

const Key = "key"
const Cloud = "cloud"
const Sem = "semaphore"
const Config = "config"
const Bootstrap = "bootstrap"

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

	// creating the connection client with Hashicorp vault, and using the keyring we created above
	// as this clients keyring. This allows the API key we added earlier to be used when calling the API
	hashiConn := hashicorp.VaultConnection{
		VaultUrl:  os.Getenv("HASHICORP_VAULT_URL"),
		HttpProto: "https",
		KeyRing:   apikeyring,
		Client:    &http.Client{},
	}
	// Since the hashicorp.VaultConnection struct implements daemon.DaemonKeyRing,
	// we can add it to the apikeyring's 'Rungs' field.
	// When calling the apikeyring's .GetKey() method, it will check its
	// internal cache for that key, and then it will attempt to find that key in
	// each rung that it has on its keyring
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)
	lnclient := linode.LinodeConnection{Client: &http.Client{}}

	/*
	   Here we are adding the Semaphore API key to the keyring and making a new semaphore client
	*/
	conf := daemon.ReadConfig("./.config.json")

	if os.Args[1] == Config {
		method := os.Args[2]
		switch method {
		case "init":
			daemon.BlankConfig("./.config.template.json")
			os.Exit(0)
		}
	}
	if os.Args[1] == Bootstrap {
		semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
		apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)

		linodeReq, err := linode.NewLinodeBodyBuilder(conf.Image(), conf.Region(), conf.LinodeType(), apikeyring)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%+v\n", linodeReq)
		newLnResp, err := lnclient.CreateNewLinode(apikeyring, linodeReq)
		if err != nil {
			log.Fatal(err)
		}
		// Seed Semaphore with details
		gitKey, err := apikeyring.GetKey(keytags.GIT_SSH_KEYNAME)
		if err != nil {
			log.Fatal(err)
		}
		keyReq := semaphoreConn.NewKeyRequestBuilder(keytags.GIT_SSH_KEYNAME, "ssh", gitKey)
		err = semaphoreConn.AddKey(keytags.GIT_SSH_KEYNAME, keyReq)
		if err != nil {
			log.Fatal(err)
		}
		err = semaphoreConn.AddRepository(conf.Repo(), conf.Branch())
		if err != nil {
			log.Fatal(err)
		}
		err = semaphoreConn.AddInventory(newLnResp.Ipv4)
		if err != nil {
			log.Fatal(err)
		}
		err = semaphoreConn.AddJobTemplate("playbook_gather_facts.yml", fmt.Sprintf("%s:%s", conf.Repo(), conf.Branch()))
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)

	}

	if os.Args[1] == Key {
		method := os.Args[2]
		switch method {
		case "tags":
			for k := range keytags.AllTags {
				fmt.Println(k)
			}
			os.Exit(0)
		case "show":
			found, err := apikeyring.GetKey(os.Args[3])
			if err != nil {
				log.Fatal("Key '", os.Args[3], "' Not found.")
			}
			fmt.Printf("%+v\n", found)
			os.Exit(0)
		case "add":
			semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
			apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)
			sshkey, err := apikeyring.GetKey(keytags.GIT_SSH_KEYNAME)
			if err != nil {
				log.Fatal(err)
			}
			err = semaphoreConn.AddKey(keytags.GIT_SSH_KEYNAME, daemon.SshKey{User: sshkey.GetPublic(), PrivateKey: sshkey.GetSecret()})
			if err != nil {
				log.Fatal(err)
			}

		}
	}
	if os.Args[1] == Sem {
		semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
		apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)
		method := os.Args[2]
		switch method {
		case "show":
			proj, err := semaphoreConn.GetProjects()
			if err != nil {
				log.Fatal("Error creating a new semaphore project: ", err)
			}
			fmt.Printf("%+v\n", proj)
			os.Exit(0)
		case "status":
			fmt.Printf("Yosai Server ID: %v\nSemaphore Upstream: %s\n", semaphoreConn.ProjectId, semaphoreConn.ServerUrl)
			os.Exit(0)
		case "add":
			err = semaphoreConn.AddJobTemplate(conf.PlaybookName(), fmt.Sprintf("%s:%s", conf.Repo(), conf.Branch()))
			if err != nil {
				log.Fatal(err)
			}
			os.Exit(0)
		case "inv":
			err = semaphoreConn.AddInventory([]string{os.Args[3]})
			if err != nil {
				log.Fatal(err)
			}
		}

	}
	if os.Args[1] == Cloud {
		method := os.Args[2]
		switch method {
		case "show":
			server, err := lnclient.GetLinode(apikeyring, os.Args[3])
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%+v\n", server)
			os.Exit(0)
		case "list":
			all, err := lnclient.ListLinodes(apikeyring)
			if err != nil {
				log.Fatal(err)
			}
			for i := range all.Data {
				fmt.Printf("ID: %v\n", all.Data[i].Id)
			}
			os.Exit(0)
		case "rm":
			err := lnclient.DeleteLinode(apikeyring, os.Args[3])
			if err != nil {
				log.Fatalf("Couldnt delete linode: %v. Error: %s", os.Args[3], err)
			}
		}
	}
}
