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
	configServer := daemon.NewConfigServerImpl("localhost:8080", "http")
	conf := configServer.Get()
	fmt.Printf("config gotten: %+v\n", conf)
	conf.SetConfigIO(configServer)
	conf.SetStreamIO(os.Stdout)
	apikeyring := daemon.NewKeyRing(conf, keytags.ConstKeytag{})
	// Here we are demonstrating how you add a key to a keyring, in this
	// case it is the top level keyring.
	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, daemon.BearerAuth{
		Secret: os.Getenv(keytags.HASHICORP_VAULT_KEYNAME),
	})
	hashiConn := hashicorp.VaultConnection{
		VaultUrl:  conf.Service.SecretsBackendUrl,
		HttpProto: "https",
		KeyRing:   apikeyring,
		Client:    &http.Client{},
	}
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)
	err = apikeyring.Bootstrap()
	if err != nil {
		log.Fatal(err)

	}
	// creating the connection client with Hashicorp vault, and using the keyring we created above
	// as this clients keyring. This allows the API key we added earlier to be used when calling the API
	lnConn := linode.LinodeConnection{Client: &http.Client{}, Keyring: apikeyring, Config: conf, KeyTagger: keytags.ConstKeytag{}}
	semaphoreConn := semaphore.NewSemaphoreClient(conf.Service.AnsibleBackendUrl, "https", apikeyring, conf, keytags.ConstKeytag{})
	apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)

	ctx := daemon.NewContext(UNIX_DOMAIN_SOCK_PATH, os.Stdout, apikeyring, conf)

	lnRouter := linode.NewLinodeRouter()
	lnRouter.Register(daemon.ADD, lnConn.AddLinodeHandler)
	lnRouter.Register(daemon.SHOW, lnConn.ShowLinodeHandler)
	lnRouter.Register(daemon.DELETE, lnConn.DeleteLinodeHandler)
	lnRouter.Register(daemon.POLL, lnConn.PollLinodeHandler)

	semHostsRouter := semaphore.NewSemaphoreRouter()
	semHostsRouter.Register(daemon.ADD, semaphoreConn.AddHostHandler)
	semHostsRouter.Register(daemon.DELETE, semaphoreConn.DeleteHostHandler)
	semHostsRouter.Register(daemon.SHOW, semaphoreConn.ShowHostHandler)

	semProjRouter := semaphore.NewSemaphoreRouter()
	semProjRouter.Register(daemon.ADD, semaphoreConn.AddProjectHandler)
	semProjRouter.Register(daemon.SHOW, semaphoreConn.ShowProjectHandler)

	semTaskRouter := semaphore.NewSemaphoreRouter()
	semTaskRouter.Register(daemon.RUN, semaphoreConn.RunTaskHandler)
	semTaskRouter.Register(daemon.POLL, semaphoreConn.PollTaskHandler)
	semTaskRouter.Register(daemon.SHOW, semaphoreConn.ShowTaskHandler)

	semBootstrapRouter := semaphore.NewSemaphoreRouter()
	semBootstrapRouter.Register(daemon.BOOTSTRAP, semaphoreConn.BootstrapHandler)

	configPeerRouter := daemon.NewConfigRouter()
	configPeerRouter.Register(daemon.ADD, conf.AddPeerHandler)
	configPeerRouter.Register(daemon.DELETE, conf.DeletePeerHandler)

	configServerRouter := daemon.NewConfigRouter()
	configServerRouter.Register(daemon.ADD, conf.AddServerHandler)
	configServerRouter.Register(daemon.DELETE, conf.DeleteServerHandler)

	configRouter := daemon.NewConfigRouter()
	configRouter.Register(daemon.SHOW, conf.ShowConfigHandler)
	configRouter.Register(daemon.SAVE, conf.SaveConfigHandler)
	configRouter.Register(daemon.RELOAD, conf.ReloadConfigHandler)

	keyringRouter := daemon.NewKeyRingRouter()
	keyringRouter.Register(daemon.SHOW, apikeyring.ShowKeyringHandler)
	keyringRouter.Register(daemon.BOOTSTRAP, apikeyring.BootstrapKeyringHandler)
	keyringRouter.Register(daemon.RELOAD, apikeyring.ReloadKeyringHandler)

	vpnRouter := daemon.NewVpnRouter()
	vpnRouter.Register(daemon.SHOW, ctx.VpnShowHandler)
	vpnRouter.Register(daemon.SAVE, ctx.VpnSaveHandler)

	ctxRouter := daemon.NewContextRouter()
	ctxRouter.Register(daemon.SHOW, ctx.ShowRoutesHandler)

	ctx.Register("cloud", lnRouter)
	ctx.Register("keyring", keyringRouter)
	ctx.Register("config", configRouter)
	ctx.Register("config-peer", configPeerRouter)
	ctx.Register("config-server", configServerRouter)
	ctx.Register("ansible", semBootstrapRouter)
	ctx.Register("ansible-hosts", semHostsRouter)
	ctx.Register("ansible-projects", semProjRouter)
	ctx.Register("ansible-task", semTaskRouter)
	ctx.Register("vpn-config", vpnRouter)
	ctx.Register("routes", ctxRouter)
	ctx.ListenAndServe()
}
