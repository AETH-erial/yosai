package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/config"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/keyring"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
	"github.com/joho/godotenv"
)

const ConfigFileName = "yosai.json"

func GetSshKeyPrompt(daemonKeyring keyring.DaemonKeyRing, conf config.Configuration) {
	fmt.Print("Enter the full path of the ssh key to use for your daemon: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	keypath := strings.Trim(line, "\n")
	pubkeyBytes, err := os.ReadFile(keypath + ".pub")
	if err != nil {
		log.Fatal("Error reading the public key: ", err)
	}
	privkeyBytes, err := os.ReadFile(keypath)
	if err != nil {
		log.Fatal("Error reading the private key: ", err)
	}
	err = daemonKeyring.AddKey(keytags.SYSTEM_SSH_KEYNAME, keyring.SshKey{
		PublicKey:  strings.Trim(string(pubkeyBytes), "\n"),
		PrivateKey: strings.Trim(string(privkeyBytes), "\n"),
		Username:   conf.Username,
	})
	if err != nil {
		log.Fatal(err.Error())
	}

}

func xdgUserHome() string {
	return path.Join("/home", os.Getenv("USER"), ".config", "yosai.json")
}

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func main() {
	configMode := flag.String("config-mode", "", "Specify the configuration mode to run the daemon as, i.e. 'server' or 'host'")
	username := flag.String("username", "", "This is the username to run the daemon as. Usually only applicable if using a configuration server")
	secretsBackendKey := flag.String("secrets-backend-key", "", "This is the API key for the secret backend")
	envFile := flag.String("env", "./.env", "Pass an environment variable file to this flag. Default's to '.env' in the CWD")
	configInit := flag.Bool("config-init", false, "pass this to create a blank config at ./.config.tmpl")
	envInit := flag.Bool("env-init", false, "pass this to create a blank env file at ./.env.tmpl")
	configServerPort := flag.Int("config-server-port", 8080, "Specify the port to reach the config server at. defaults to 8080")
	configServerAddr := flag.String("config-server-addr", "localhost", "Specify the network address to reach the config server at. Defaults to localhost")
	configServerProto := flag.String("config-server-proto", "http", "Specify the protocol to contact the config server with, e.g. http,https,yosai. Defaults to http")
	configFileLoc := flag.String("config-file-loc", xdgUserHome(), "Pass a configuration file in a non-default location. Defaults to /home/$USER/.config/yosai.json")
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
	err := godotenv.Load(*envFile)
	if err != nil {
		fmt.Println("Couldn't find an env file in the current directory. Relying on program startup sequence to provide initial configuration")
	}
	startupArgs := map[config.StartupArgKeyname]string{
		config.ConfigModeArg:        *configMode,
		config.UsernameArg:          *username,
		config.SecretsBackendKeyArg: *secretsBackendKey,
		config.ConfigServerAddr:     *configServerAddr,
		config.ConfigServerProto:    *configServerProto,
		config.ConfigFileLoc:        *configFileLoc,
	}
	var configIO config.DaemonConfigIO
	startupData := config.Turnkey(startupArgs)
	switch startupData.ConfigurationMode {
	case "server":
		configIO = config.NewConfigServerImpl(startupData.ConfigServerAddr, *configServerPort, startupData.ConfigServerProto)
	case "host":
		configIO = config.NewConfigHostImpl(startupData.ConfigFileLoc)
	default:
		fmt.Println("unknown configuration mode: ", startupData.ConfigurationMode, " passed.")
		os.Exit(199)
	}
	conf := config.NewConfiguration(os.Stdout, config.ValidateUsername(startupData.Username))
	configIO.Propogate(conf)
	conf.SetConfigIO(configIO)
	conf.SetStreamIO(os.Stdout)
	apikeyring := keyring.NewKeyRing(conf, keytags.ConstKeytag{})
	// Here we are demonstrating how you add a key to a keyring, in this
	// case it is the top level keyring.
	apikeyring.AddKey(keytags.HASHICORP_VAULT_KEYNAME, keyring.BearerAuth{
		Secret:   startupData.SecretsBackendKey,
		Username: config.ValidateUsername(startupData.Username),
	})
	hashiConn := hashicorp.VaultConnection{
		VaultUrl:  conf.Service.SecretsBackendUrl,
		HttpProto: "https",
		KeyRing:   apikeyring,
		Client:    &http.Client{},
	}
	apikeyring.Rungs = append(apikeyring.Rungs, hashiConn)
	_, err = apikeyring.GetKey(keytags.SYSTEM_SSH_KEYNAME)
	if err != nil {
		GetSshKeyPrompt(hashiConn, *conf)
	}
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
