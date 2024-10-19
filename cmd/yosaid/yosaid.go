package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/config"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/keyring"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func main() {
	configInit := flag.Bool("config-init", false, "pass this to create a blank config at ./.config.tmpl")
	envInit := flag.Bool("env-init", false, "pass this to create a blank env file at ./.env.tmpl")
	flag.Parse()
	if *configInit {
		err := config.BlankConfig("./.config.tmpl")
		if err != nil {
			log.Fatal("Couldnt create blank config: ", err)

		}
		fmt.Println("Blank config created at ./.config.tmpl")
		os.Exit(0)
	}
	if *envInit {
		err := config.BlankEnv("./.env.tmpl")
		if err != nil {
			log.Fatal("Couldnt create blank env file: ", err)
		}
		fmt.Println("Blank env file created at ./.env.tmpl")
		os.Exit(0)
	}
	err := config.LoadAndVerifyEnv("./.env", config.EnvironmentVariables)
	if err != nil {
		log.Fatal("Error loading env file: ", err)
	}
	configServer := config.NewConfigServerImpl("192.168.50.35:8080", "http")
	conf := config.NewConfiguration(os.Stdout, "aeth")
	configServer.Propogate(conf)
	fmt.Printf("config gotten: %+v\n", conf)
	conf.SetConfigIO(configServer)
	conf.SetStreamIO(os.Stdout)
	apikeyring := keyring.NewKeyRing(conf, keytags.ConstKeytag{})
	// Here we are demonstrating how you add a key to a keyring, in this
	// case it is the top level keyring.
	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, keyring.BearerAuth{
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
	lnRouter.Register(daemonproto.ADD, lnConn.AddLinodeHandler)
	lnRouter.Register(daemonproto.SHOW, lnConn.ShowLinodeHandler)
	lnRouter.Register(daemonproto.DELETE, lnConn.DeleteLinodeHandler)
	lnRouter.Register(daemonproto.POLL, lnConn.PollLinodeHandler)

	semHostsRouter := semaphore.NewSemaphoreRouter()
	semHostsRouter.Register(daemonproto.ADD, semaphoreConn.AddHostHandler)
	semHostsRouter.Register(daemonproto.DELETE, semaphoreConn.DeleteHostHandler)
	semHostsRouter.Register(daemonproto.SHOW, semaphoreConn.ShowHostHandler)

	semProjRouter := semaphore.NewSemaphoreRouter()
	semProjRouter.Register(daemonproto.ADD, semaphoreConn.AddProjectHandler)
	semProjRouter.Register(daemonproto.SHOW, semaphoreConn.ShowProjectHandler)

	semTaskRouter := semaphore.NewSemaphoreRouter()
	semTaskRouter.Register(daemonproto.RUN, semaphoreConn.RunTaskHandler)
	semTaskRouter.Register(daemonproto.POLL, semaphoreConn.PollTaskHandler)
	semTaskRouter.Register(daemonproto.SHOW, semaphoreConn.ShowTaskHandler)

	semBootstrapRouter := semaphore.NewSemaphoreRouter()
	semBootstrapRouter.Register(daemonproto.BOOTSTRAP, semaphoreConn.BootstrapHandler)

	configPeerRouter := config.NewConfigRouter()
	configPeerRouter.Register(daemonproto.ADD, conf.AddPeerHandler)
	configPeerRouter.Register(daemonproto.DELETE, conf.DeletePeerHandler)

	configServerRouter := config.NewConfigRouter()
	configServerRouter.Register(daemonproto.ADD, conf.AddServerHandler)
	configServerRouter.Register(daemonproto.DELETE, conf.DeleteServerHandler)

	configRouter := config.NewConfigRouter()
	configRouter.Register(daemonproto.SHOW, conf.ShowConfigHandler)
	configRouter.Register(daemonproto.SAVE, conf.SaveConfigHandler)
	configRouter.Register(daemonproto.RELOAD, conf.ReloadConfigHandler)

	keyringRouter := keyring.NewKeyRingRouter()
	keyringRouter.Register(daemonproto.SHOW, apikeyring.ShowKeyringHandler)
	keyringRouter.Register(daemonproto.BOOTSTRAP, apikeyring.BootstrapKeyringHandler)
	keyringRouter.Register(daemonproto.RELOAD, apikeyring.ReloadKeyringHandler)

	vpnRouter := daemon.NewVpnRouter()
	vpnRouter.Register(daemonproto.SHOW, ctx.VpnShowHandler)
	vpnRouter.Register(daemonproto.SAVE, ctx.VpnSaveHandler)

	ctxRouter := daemon.NewContextRouter()
	ctxRouter.Register(daemonproto.SHOW, ctx.ShowRoutesHandler)

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
