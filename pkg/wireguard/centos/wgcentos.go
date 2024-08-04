package wg

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"text/template"
)

const (
	ExitSetupSuccess = 0
	ExitSetupFailed  = 1
)

const (
	ENV_WG_TUN_FD             = "WG_TUN_FD"
	ENV_WG_UAPI_FD            = "WG_UAPI_FD"
	ENV_WG_PROCESS_FOREGROUND = "WG_PROCESS_FOREGROUND"
)

//go:embed wireguard.conf.templ
var confTmpl string

type WireguardTemplateSeed struct {
	VpnClientPrivateKey string
	VpnClientAddress    string
	Peers               []WireguardTemplatePeer
}

type WireguardTemplatePeer struct {
	Pubkey  string
	Address string
	Port    int
}

/*
Render out a client configuration file, utilizing the data provided from Semaphore and the daemon keyring

	    :param tmpl: a template.Template that will be populated with the VPN data
		:param wgData: a WireGuardTemplateSeed struct that contains all the info needed to populate a wireguard config file
*/
func RenderClientConfiguration(wgData WireguardTemplateSeed) ([]byte, error) {
	buff := bytes.NewBuffer([]byte{})
	tmpl, err := template.New("wireguard.conf.templ").Parse(confTmpl)
	if err != nil {
		log.Fatal(err)
	}

	err = tmpl.Execute(buff, wgData)
	if err != nil {
		return buff.Bytes(), &TemplatingError{TemplateData: wgData, Msg: err.Error()}
	}

	return buff.Bytes(), nil
}

/*
Start a wireguard interface
*/
func StartWgInterface(intfName string) ([]byte, error) {
	wgCmd := exec.Command("wg-quick", "up", intfName)
	return wgCmd.Output()
	/*
	   var foreground bool

	   	if !foreground {
	   		foreground = os.Getenv(ENV_WG_PROCESS_FOREGROUND) == "1"
	   	}

	   // get log level (default: info)

	   	logLevel := func() int {
	   		switch os.Getenv("LOG_LEVEL") {
	   		case "verbose", "debug":
	   			return device.LogLevelVerbose
	   		case "error":
	   			return device.LogLevelError
	   		case "silent":
	   			return device.LogLevelSilent
	   		}
	   		return device.LogLevelError
	   	}()

	   // open TUN device (or use supplied fd)

	   	tdev, err := func() (tun.Device, error) {
	   		tunFdStr := os.Getenv(ENV_WG_TUN_FD)
	   		if tunFdStr == "" {
	   			return tun.CreateTUN(intfName, device.DefaultMTU)
	   		}

	   		// construct tun device from supplied fd

	   		fd, err := strconv.ParseUint(tunFdStr, 10, 32)
	   		if err != nil {
	   			return nil, err
	   		}

	   		err = unix.SetNonblock(int(fd), true)
	   		if err != nil {
	   			return nil, err
	   		}

	   		file := os.NewFile(uintptr(fd), "")
	   		return tun.CreateTUNFromFile(file, device.DefaultMTU)
	   	}()

	   	if err == nil {
	   		realInterfaceName, err2 := tdev.Name()
	   		if err2 == nil {
	   			intfName = realInterfaceName
	   		}
	   	}

	   logger := device.NewLogger(

	   	logLevel,
	   	fmt.Sprintf("(%s) ", intfName),

	   )

	   	if err != nil {
	   		logger.Errorf("Failed to create TUN device: %v", err)
	   		os.Exit(ExitSetupFailed)
	   	}

	   // open UAPI file (or use supplied fd)

	   	fileUAPI, err := func() (*os.File, error) {
	   		uapiFdStr := os.Getenv(ENV_WG_UAPI_FD)
	   		if uapiFdStr == "" {
	   			return ipc.UAPIOpen(intfName)
	   		}

	   		// use supplied fd

	   		fd, err := strconv.ParseUint(uapiFdStr, 10, 32)
	   		if err != nil {
	   			return nil, err
	   		}

	   		return os.NewFile(uintptr(fd), ""), nil
	   	}()

	   	if err != nil {
	   		logger.Errorf("UAPI listen error: %v", err)
	   		os.Exit(ExitSetupFailed)
	   		return
	   	}

	   // daemonize the process

	   	if !foreground {
	   		env := os.Environ()
	   		env = append(env, fmt.Sprintf("%s=3", ENV_WG_TUN_FD))
	   		env = append(env, fmt.Sprintf("%s=4", ENV_WG_UAPI_FD))
	   		env = append(env, fmt.Sprintf("%s=1", ENV_WG_PROCESS_FOREGROUND))
	   		files := [3]*os.File{}
	   		if os.Getenv("LOG_LEVEL") != "" && logLevel != device.LogLevelSilent {
	   			files[0], _ = os.Open(os.DevNull)
	   			files[1] = os.Stdout
	   			files[2] = os.Stderr
	   		} else {
	   			files[0], _ = os.Open(os.DevNull)
	   			files[1], _ = os.Open(os.DevNull)
	   			files[2], _ = os.Open(os.DevNull)
	   		}
	   		attr := &os.ProcAttr{
	   			Files: []*os.File{
	   				files[0], // stdin
	   				files[1], // stdout
	   				files[2], // stderr
	   				tdev.File(),
	   				fileUAPI,
	   			},
	   			Dir: ".",
	   			Env: env,
	   		}

	   		path, err := os.Executable()
	   		if err != nil {
	   			logger.Errorf("Failed to determine executable: %v", err)
	   			os.Exit(ExitSetupFailed)
	   		}

	   		process, err := os.StartProcess(
	   			path,
	   			os.Args,
	   			attr,
	   		)
	   		if err != nil {
	   			logger.Errorf("Failed to daemonize: %v", err)
	   			os.Exit(ExitSetupFailed)
	   		}
	   		process.Release()
	   		return
	   	}

	   device := device.NewDevice(tdev, conn.NewDefaultBind(), logger)

	   logger.Verbosef("Device started")

	   errs := make(chan error)
	   term := make(chan os.Signal, 1)

	   uapi, err := ipc.UAPIListen(intfName, fileUAPI)

	   	if err != nil {
	   		logger.Errorf("Failed to listen on uapi socket: %v", err)
	   		os.Exit(ExitSetupFailed)
	   	}

	   	go func() {
	   		for {
	   			conn, err := uapi.Accept()
	   			if err != nil {
	   				errs <- err
	   				return
	   			}
	   			go device.IpcHandle(conn)
	   		}
	   	}()

	   logger.Verbosef("UAPI listener started")

	   // wait for program to terminate

	   signal.Notify(term, unix.SIGTERM)
	   signal.Notify(term, os.Interrupt)

	   select {
	   case <-term:
	   case <-errs:
	   case <-device.Wait():
	   }

	   // clean up

	   uapi.Close()
	   device.Close()

	   logger.Verbosef("Shutting down")
	*/
}

type TemplatingError struct {
	TemplateData WireguardTemplateSeed
	Msg          string
}

func (t *TemplatingError) Error() string {
	return fmt.Sprintf("There was an error executing the template file: %s, Seed data: %+v\n", t.Msg, t.TemplateData)
}
