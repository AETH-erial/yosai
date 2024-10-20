package dclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"git.aetherial.dev/aeth/yosai/pkg/cloud/linode"
	"git.aetherial.dev/aeth/yosai/pkg/config"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/hashicorp"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/keyring"
	"git.aetherial.dev/aeth/yosai/pkg/semaphore"
)

type DaemonClient struct {
	SockPath string // the absolute path of the unix domain socket
	Stream   io.ReadWriter
}

const BLANK_JSON = "{\"blank\": \"hey!\"}"

/*
Gets the configuration from the upstream daemonproto/server
*/
func (d DaemonClient) GetConfig() config.Configuration {

	resp := d.Call([]byte(BLANK_JSON), "config", "show")
	cfg := config.Configuration{}
	err := json.Unmarshal(resp.Body, &cfg)
	if err != nil {
		log.Fatal("error unmarshalling config struct ", err.Error())
	}
	return cfg

}

/*
Parse a string into a hashmap to allow for key-based data retrieval. Strings formatted in a
comma delimited, key-value pair denoted by an equal sign can be parsed. i.e. 'name=primary,wan=8.8.8.8'
:param arg: the string to be parsed
*/
func makeArgMap(arg string) map[string]string {
	argSplit := strings.Split(arg, ",")
	argMap := map[string]string{}
	for i := range argSplit {
		a := strings.SplitN(strings.TrimSpace(argSplit[i]), "=", 2)
		fmt.Println(a)
		/*
			if len(a) != 2 {
				log.Fatal("Key values must be passed comma delimmited, seperated with an '='. i.e. 'public=12345abcdef,secret=qwerty69420'")
			}
		*/
		argMap[strings.TrimSpace(strings.ToLower(a[0]))] = strings.TrimSpace(a[1])
	}
	return argMap
}

/*
Take an argument string of comma-delimmited k/v pairs (denoted with an '=' sign and
turn it into a hashmap, and then pack it into a JSON byte array

	:param msg: the message string to parse
*/
func Pack(msg string) []byte {
	b, err := json.Marshal(makeArgMap(msg))
	if err != nil {
		log.Fatal("Fatal problem trying to marshal the message: ", msg, " into a JSON string: ", err.Error())
	}
	return b
}

/*
Convenience function for building a request to add a server to the
daemonproto's configuration table

	:param argMap: a map with named elements that correspond to the subsequent struct's fields
*/
func serverAddRequestBuilder(argMap map[string]string) []byte {
	port, err := strconv.Atoi(argMap["port"])
	if err != nil {
		log.Fatal("Port passed: ", argMap["port"], " is not a valid integer.")
	}
	if port <= 0 || port > 65535 {
		log.Fatal("Port passed: ", port, " Was not in the valid range of between 1-65535.")
	}

	b, _ := json.Marshal(config.VpnServer{WanIpv4: argMap["wan"], Name: argMap["name"]})

	return b
}

/*
Wraps the creation of a request to add/delete a peer from the config

	:param argMap: a map with named elements that correspond to the subsequent struct's fields
*/
func peerAddRequestBuilder(argMap map[string]string) []byte {
	b, _ := json.Marshal(config.VpnClient{Name: argMap["name"], Pubkey: argMap["pubkey"]})
	return b
}

/*
Wraps the creation of a request to add to the keyring
	:param argMap: a map with named elements that correspond to the subsequent struct's fields
*/

func keyringRequstBuilder(argMap map[string]string) []byte {
	b, _ := json.Marshal(hashicorp.VaultItem{Public: argMap["public"], Secret: argMap["secret"], Type: keyring.AssertKeytype(argMap["type"]), Name: argMap["name"]})
	return b

}

/*
Wraps the creation of a request to render a configuration
    :param argMap: a map with named elements that correspond to the subsequent struct's fields
*/

func configRenderRequestBuilder(argMap map[string]string) []byte {
	b, _ := json.Marshal(daemon.ConfigRenderRequest{Server: argMap["server"], Client: argMap["client"]})
	return b
}

func (d DaemonClient) addLinodeRequestBuilder(arg string) []byte {
	cfg := d.GetConfig()
	addLn := linode.AddLinodeRequest{
		Name:   arg,
		Image:  cfg.Cloud.Image,
		Type:   cfg.Cloud.LinodeType,
		Region: cfg.Cloud.Region,
	}
	b, _ := json.Marshal(addLn)
	return b

}

func (d DaemonClient) Call(payload []byte, target string, method string) daemonproto.SockMessage {
	msg := daemonproto.SockMessage{
		Type:       daemonproto.MsgRequest,
		TypeLen:    int8(len(daemonproto.MsgRequest)),
		StatusMsg:  "",
		StatusCode: 0,
		Version:    daemonproto.SockMsgVers,
		Body:       payload,
		Target:     target,
		Method:     method,
	}
	conn, err := net.Dial("unix", d.SockPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	buf := bytes.NewBuffer(daemonproto.Marshal(msg))
	_, err = io.Copy(conn, buf)
	if err != nil {
		log.Fatal("write error:", err)
	}
	resp := bytes.NewBuffer([]byte{})
	_, err = io.Copy(resp, conn)
	if err != nil {
		if err == io.EOF {
			fmt.Println("exited ok.")
			os.Exit(0)
		}
		log.Fatal(err)
	}
	return daemonproto.Unmarshal(resp.Bytes())

}

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

/*
Build a JSON request to send the yosaid daemonproto

	    :param v: a struct to serialize for a request
		:param value: a string to put into the request
*/
func jsonBuilder(v interface{}, value string) []byte {
	delLn, ok := v.(linode.DeleteLinodeRequest)
	if ok {
		delLn = linode.DeleteLinodeRequest{
			Name: value,
		}
		b, _ := json.Marshal(delLn)
		return b

	}
	pollLn, ok := v.(linode.PollLinodeRequest)
	if ok {
		pollLn = linode.PollLinodeRequest{
			Address: value,
		}
		b, _ := json.Marshal(pollLn)
		return b
	}
	semReq, ok := v.(semaphore.SemaphoreRequest)
	if ok {
		semReq = semaphore.SemaphoreRequest{
			Target: value,
		}
		b, _ := json.Marshal(semReq)
		return b

	}
	return []byte("{\"data\":\"test\"}")

}

/*
Create a server, and propogate it across the daemonproto's system
*/
func (d DaemonClient) NewServer(name string) error {
	// create new server in cloud environment
	ipv4, err := d.addLinode(name)
	if err != nil {
		return err
	}
	// add server data to daemonproto configuration
	b, _ := json.Marshal(config.VpnServer{WanIpv4: ipv4, Name: name})
	resp := d.Call(b, "config-server", "add")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}

	// add configuration data to ansible
	resp = d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, name), "ansible-hosts", "add")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil
}

/*
Helper function to get servers from the daemonproto config

	:param val: either the WAN IPv4 address, or the name of the server to get
*/
func (d DaemonClient) GetServer(val string) (config.VpnServer, error) {
	cfg := d.GetConfig()
	for name := range cfg.Service.Servers {
		if cfg.Service.Servers[name].WanIpv4 == val {
			return cfg.Service.Servers[val], nil
		}
		server, ok := cfg.Service.Servers[val]
		if ok {
			return server, nil
		}
	}
	return config.VpnServer{}, &ServerNotFound{Name: val}

}

/*
Add a server to the configuration
*/
func (d DaemonClient) AddServeToConfig(val string) error {
	argMap := makeArgMap(val)
	resp := d.Call(serverAddRequestBuilder(argMap), "config-server", "add")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil
}

/*
Trigger the daemonproto to execute the vpn rotation playbook on all of the servers in the ansible inventory
*/
func (d DaemonClient) ConfigureServers() (daemonproto.SockMessage, error) {
	resp := d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, semaphore.YosaiVpnRotationJob), "ansible-job", "run")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return resp, &DaemonClientError{SockMsg: resp}
	}
	taskInfo := semaphore.TaskInfo{}
	err := json.Unmarshal(resp.Body, &taskInfo)
	if err != nil {
		return resp, &DaemonClientError{SockMsg: resp}
	}
	resp = d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, fmt.Sprint(taskInfo.ID)), "ansible-job", "poll")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return resp, &DaemonClientError{SockMsg: resp}
	}
	return resp, nil

}

/*
Poll until a server is done being created

	:param name: the name of the server
*/
func (d DaemonClient) PollServer(name string) (daemonproto.SockMessage, error) {
	resp := d.Call(jsonBuilder(linode.PollLinodeRequest{}, name), "cloud", "poll")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return resp, &DaemonClientError{SockMsg: resp}
	}
	return resp, nil

}

/*
Remove a server from the daemonproto configuration

	:param name: the name of the server to remove
*/
func (d DaemonClient) RemoveServerFromConfig(name string) error {
	b, _ := json.Marshal(config.VpnServer{Name: name})
	resp := d.Call(b, "config-server", "delete")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil
}

/*
Remove a server from the ansible inventory

	:param name: the name of the server to remove from ansible
*/
func (d DaemonClient) RemoveServerFromAnsible(name string) error {
	server, err := d.GetServer(name)
	if err != nil {
		return err
	}
	resp := d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, server.WanIpv4), "ansible-hosts", "delete")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil

}

/*
Destroy a server by its logical name in the configuration, ansible inventory, and cloud provider

	:param name: the name of the server in the system
*/
func (d DaemonClient) DestroyServer(name string) error {
	cfg := d.GetConfig()
	resp := d.Call(jsonBuilder(linode.DeleteLinodeRequest{}, name), "cloud", "delete")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	resp = d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, cfg.Service.Servers[name].WanIpv4), "ansible-hosts", "delete")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	b, _ := json.Marshal(config.VpnServer{Name: name})
	resp = d.Call(b, "config-server", "delete")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil

}

func (d DaemonClient) HealthCheck() (daemonproto.SockMessage, error) {
	return daemonproto.SockMessage{}, nil
}
func (d DaemonClient) LockFirewall() error {
	return nil
}

/*
Render the a wireguard configuration file
*/
func (d DaemonClient) RenderWgConfig(arg string) daemonproto.SockMessage {
	argMap := makeArgMap(arg)

	b, _ := json.Marshal(daemon.ConfigRenderRequest{Server: argMap["server"], Client: argMap["client"]})
	return d.Call(b, "vpn-config", "show")
}

/*
Render the a wireguard configuration file
*/
func (d DaemonClient) SaveWgConfig(arg string) daemonproto.SockMessage {
	argMap := makeArgMap(arg)

	b, _ := json.Marshal(daemon.ConfigRenderRequest{Server: argMap["server"], Client: argMap["client"]})
	return d.Call(b, "vpn-config", "save")
}

/*
This is function creates a linode, and then returns the IPv4 as a string

	:param name: the name to assign the linode
*/
func (d DaemonClient) addLinode(name string) (string, error) {
	cfg := d.GetConfig()
	b, _ := json.Marshal(linode.AddLinodeRequest{
		Image:  cfg.Cloud.Image,
		Region: cfg.Cloud.Region,
		Type:   cfg.Cloud.LinodeType,
		Name:   name,
	})
	resp := d.Call(b, "cloud", "add")
	linodeResp := linode.GetLinodeResponse{}
	err := json.Unmarshal(resp.Body, &linodeResp)
	if err != nil {
		return "", &DaemonClientError{SockMsg: resp}
	}
	return linodeResp.Ipv4[0], nil
}

func (d DaemonClient) BootstrapAll() error {
	resp := d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, "all"), "ansible", "bootstrap")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil
}

/*
Force the daemonproto to reload its configuration
*/
func (d DaemonClient) ForceReload() error {
	resp := d.Call([]byte(BLANK_JSON), "config", "reload")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil
}

/*
Force a configuration save to the daemonproto/server
*/
func (d DaemonClient) ForceSave() error {
	resp := d.Call([]byte(BLANK_JSON), "config", "save")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil

}

func (d DaemonClient) ShowAllRoutes() daemonproto.SockMessage {
	return d.Call([]byte(BLANK_JSON), "routes", "show")

}

/*
This creates a new server, wrapping the DaemonClient.NewServer() function, and then configures it

	:param name: the name to give the server
*/
func (d DaemonClient) ServiceInit(name string) error {
	d.NewServer(name)
	resp := d.Call(jsonBuilder(linode.PollLinodeRequest{}, name), "cloud", "poll")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	resp = d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, "VPN Rotation playbook"), "ansible-job", "run")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	semaphoreResp := semaphore.TaskInfo{}
	err := json.Unmarshal(resp.Body, &semaphoreResp)
	if err != nil {
		return &DaemonClientError{SockMsg: resp}
	}
	resp = d.Call(jsonBuilder(semaphore.SemaphoreRequest{}, fmt.Sprint(semaphoreResp.ID)), "ansible-job", "poll")
	if resp.StatusCode != daemonproto.REQUEST_OK {
		return &DaemonClientError{SockMsg: resp}
	}
	return nil
}

type DaemonClientError struct {
	SockMsg daemonproto.SockMessage
}

func (d *DaemonClientError) Error() string {
	return fmt.Sprintf("Response Code: %v, Response Message: %s, Body: %s", d.SockMsg.StatusCode, d.SockMsg.StatusMsg, string(d.SockMsg.Body))

}

type ServerNotFound struct {
	Name string
}

func (s *ServerNotFound) Error() string {
	return "Server with name: " + s.Name + " was not found."
}
