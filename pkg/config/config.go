package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"github.com/joho/godotenv"
)

const LogMsgTmpl = "YOSAI Daemon ||| time: %s ||| %s\n"

var EnvironmentVariables = []string{
	"HASHICORP_VAULT_KEY",
}

const DefaultConfigLoc = "./.config.json"

type DaemonConfigIO interface {
	Propogate(*Configuration)
	Save(Configuration) error
}

/*
TODO:
- Make an implementation for using a configuration server, like a database or maybe a custom API
- Have only 1 SSH key secret type in the keyring, and propogate it into the other systems more intelligently
- intelligent keyring bootstrapping
    [KEYS]
		- read a SSH key for the VPS_SSH_KEY key
		- read in another SSH key for the GIT_SSH_KEY incase theyre different
		- Cloud API key
		- Secrets API key
		- Ansible API key
		- VPS Root
			- credentials
			- ssh key
		- VPS service account
			- credentials
			- ssh key

    [SERVICES]
    In order of priority:
		- Environment variables
		- configuration file
		- configuration server
		- user supplied input

- Create a configuration server with REST API, grabs config based on
    - LDAP
	- Local
	Configuration server should have a web UI that allows to create a config via a form, and then
	save it to that users account.

I would also like to get Hashicorp vault working with LDAP, so that I can build a large portion of this
around LDAP. Instead of needing to supply the root token to HCV, I can have the client use their LDAP credentials
all around, in HCV, the config server, and semaphore.


What else needs to be done to allow for seemless experience for a 'user' across clients?
- No on-system stored data
- Easy bootstrapping
*/

/*
Loads in the environment variable file at path, and then validates that all values in vars is present

	    :param path: the path to the .env file
		:param vars: the list of variables to check were loaded by godotenv.Load()
*/
func LoadAndVerifyEnv(path string, vars []string) error {

	err := godotenv.Load(".env")
	if err != nil {
		return err
	}
	var missing []string
	for i := range vars {
		val := os.Getenv(vars[i])
		if val == "" {
			missing = append(missing, vars[i])
		}
	}
	if len(missing) != 0 {
		return &EnvironmentVariableNotSet{Vars: missing}
	}
	return nil

}

type EnvironmentVariableNotSet struct {
	Vars []string
}

func (e *EnvironmentVariableNotSet) Error() string {
	return fmt.Sprintf("Environment variables: %v not set!", e.Vars)
}
func BlankEnv(path string) error {
	var data string
	for i := range EnvironmentVariables {
		data = data + fmt.Sprintf("%s=\n", EnvironmentVariables[i])
	}
	return os.WriteFile(path, []byte(data), 0666)

}

/*
Wrapping the add peer functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) AddPeerHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var peer VpnClient
	err := json.Unmarshal(msg.Body, &peer)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	addr, err := c.GetAvailableVpnIpv4()
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Client: "+c.AddClient(addr, peer.Pubkey, peer.Name)+" Successfully added."))
}

/*
Wrapping the delete peer functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) DeletePeerHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var req VpnClient
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	peer, err := c.GetClient(req.Name)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}

	delete(c.Service.Clients, peer.Name)
	err = c.FreeAddress(peer.VpnIpv4.String())
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Client: "+peer.Name+" Successfully deleted from the config."))
}

/*
Wrapping the add server functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) AddServerHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var req VpnServer
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	addr, err := c.GetAvailableVpnIpv4()
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	name := c.AddServer(addr, req.Name, req.WanIpv4)
	c.Log("address: ", addr.String(), "name:", name)
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Server: "+name+" Successfully added."))
}

/*
Wrapping the delete server functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) DeleteServerHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var req VpnServer
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	server, err := c.GetServer(req.Name)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}

	delete(c.Service.Servers, server.Name)
	err = c.FreeAddress(server.VpnIpv4.String())
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Server: "+server.Name+" Successfully deleted from the config."))
}

/*
Wrapping the show config functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) ShowConfigHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	b, err := json.MarshalIndent(&c, "", "   ")
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, b)
}

/*
Wrapping the save config functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) SaveConfigHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	err := c.cfgIO.Save(*c)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Configuration saved successfully."))
}

/*
Wrapping the reload config functionality in a router friendly interface

	:param msg: a message to be parsed from the daemonproto socket
*/
func (c *Configuration) ReloadConfigHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	c.cfgIO.Propogate(c)
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Configuration reloaded successfully."))
}

type ConfigRouter struct {
	routes map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage
}

func (c *ConfigRouter) Register(method daemonproto.Method, callable func(daemonproto.SockMessage) daemonproto.SockMessage) {
	c.routes[method] = callable
}

func (c *ConfigRouter) Routes() map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage {
	return c.routes
}

func NewConfigRouter() *ConfigRouter {
	return &ConfigRouter{routes: map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage{}}
}

type Username string

func ValidateUsername(name string) Username {
	return Username(name)
}

type User struct {
	Name Username
	Id   int
}

type Configuration struct {
	stream   io.Writer
	cfgIO    DaemonConfigIO
	Username Username      `json:"username"`
	Cloud    cloudConfig   `json:"cloud"`
	Ansible  ansibleConfig `json:"ansible"`
	Service  serviceConfig `json:"service"`
	HostInfo hostInfo      `json:"host_info"`
}

type hostInfo struct {
	WireguardSavePath string `json:"wireguard_save_path"`
}

type ansibleConfig struct {
	Repo         string `json:"repo_url"`
	Branch       string `json:"branch"`
	PlaybookName string `json:"playbook_name"`
}

type serviceConfig struct {
	Servers           map[string]VpnServer `json:"servers"`
	Clients           map[string]VpnClient `json:"clients"`
	VpnAddressSpace   net.IPNet            `json:"vpn_address_space"`
	VpnAddresses      map[string]bool      `json:"vpn_addresses"` // Each key is a IPv4 in the VPN, and its corresponding value is what denotes if its in use or not. False == 'In use', True == 'available'
	VpnMask           int                  `json:"vpn_mask"`      // The mask of the VPN
	VpnServerPort     int                  `json:"vpn_server_port"`
	SecretsBackend    string               `json:"secrets_backend"`
	SecretsBackendUrl string               `json:"secrets_backend_url"`
	AnsibleBackend    string               `json:"ansible_backend"`
	AnsibleBackendUrl string               `json:"ansible_backend_url"`
}

func (c *Configuration) GetServer(name string) (VpnServer, error) {
	server, ok := c.Service.Servers[name]
	if ok {
		return server, nil
	}
	for _, server := range c.Service.Servers {
		if server.Name == name {
			return server, nil
		}
	}
	return VpnServer{}, &ServerNotFound{}

}

func (c *Configuration) GetClient(name string) (VpnClient, error) {
	client, ok := c.Service.Clients[name]
	if ok {
		return client, nil
	}
	for _, client := range c.Service.Clients {
		if client.Name == name {
			return client, nil
		}
	}
	return VpnClient{}, &ServerNotFound{}

}

/*
Add a VPN server to the Service configuration

	:param server: a VpnServer struct modeling the data that comprises of a VPN server
*/
func (c *Configuration) AddServer(addr net.IP, name string, wan string) string {
	server, ok := c.Service.Servers[name]
	var serverLabel string
	if ok {
		serverLabel = c.resolveName(server.Name, name)
	} else {
		serverLabel = name
	}
	c.Service.Servers[serverLabel] = VpnServer{Name: serverLabel, WanIpv4: wan, VpnIpv4: addr}
	return serverLabel

}

type VpnClient struct {
	Name    string `json:"name"`
	VpnIpv4 net.IP `json:"vpn_ipv4"`
	Pubkey  string `json:"pubkey"`
	Default bool   `json:"default"`
}

type VpnServer struct {
	Name    string `json:"name"`     // this Label is what is used to index that server and its data within the Daemons model of the VPN environment
	WanIpv4 string `json:"wan_ipv4"` // Public IPv4
	VpnIpv4 net.IP `json:"vpn_ipv4"` // the IP address that the server will occupy on the network
}

/*
Retrieve an available IPv4 from the VPN's set address space. Returns an error if an internal address cant be
parsed to a valid IPv4, or if there are no available addresses left.
*/
func (c *Configuration) GetAvailableVpnIpv4() (net.IP, error) {
	for addr, used := range c.Service.VpnAddresses {
		if !used {
			parsedAddr := net.ParseIP(addr)
			if parsedAddr == nil {
				return nil, &VpnAddressSpaceError{Msg: "Address: " + addr + " couldnt be parsed into a valid IPv4"}
			}
			c.Service.VpnAddresses[parsedAddr.String()] = true
			return parsedAddr, nil
		}
	}
	return nil, &VpnAddressSpaceError{Msg: "No open addresses available in the current VPN address space!"}

}

/*
Return all of the clients from the client list
*/
func (c *Configuration) VpnClients() []VpnClient {
	clients := []VpnClient{}
	for _, val := range c.Service.Clients {
		clients = append(clients, val)
	}
	return clients
}

/*
Get the default VPN client
*/
func (c *Configuration) DefaultClient() (VpnClient, error) {
	for name := range c.Service.Clients {
		if c.Service.Clients[name].Default {
			return c.Service.Clients[name], nil
		}
	}
	return VpnClient{}, &ConfigError{Msg: "No default client was specified!"}
}

/*
resolve naming collision in the client list

	:param existingName: the name of the existing client in the client list
	:param dupeName: the desired name of the client to be added
*/
func (c *Configuration) resolveName(existingName string, dupeName string) string {
	incr, err := strconv.Atoi(strings.Trim(existingName, dupeName))
	if err != nil {
		c.Log("Name: ", existingName, "in the client list broke naming convention.")
		return dupeName + "0"
	}
	return fmt.Sprintf("%s%v", dupeName, incr+1)

}

/*
Register a client as a VPN client. This configuration will be propogated into server configs, so that they may connect

	    :param addr: a net.IP gotten from GetAvailableVpnIpv4()
		:param pubkey: the Wireguard public key
		:param name: the name/label of this client
*/
func (c *Configuration) AddClient(addr net.IP, pubkey string, name string) string {
	client, ok := c.Service.Clients[name]
	var clientLabel string
	if ok {
		clientLabel = c.resolveName(client.Name, name)
	} else {
		clientLabel = name
	}
	c.Service.Clients[name] = VpnClient{Name: clientLabel, Pubkey: pubkey, VpnIpv4: addr}
	return clientLabel
}

/*
Frees up an address to be used
*/
func (c *Configuration) FreeAddress(addr string) error {
	_, ok := c.Service.VpnAddresses[addr]
	if !ok {
		return &VpnAddressSpaceError{Msg: "Address: " + addr + " is not in the designated VPN Address space."}
	}
	c.Service.VpnAddresses[addr] = false
	return nil
}

/*
Get all of the in use addresses for the VPN
*/
func (c *Configuration) AllVpnAddresses() []net.IP {
	addrs := []net.IP{}
	for i := range c.Service.Servers {
		addrs = append(addrs, c.Service.Servers[i].VpnIpv4)
	}
	for i := range c.Service.Clients {
		addrs = append(addrs, c.Service.Clients[i].VpnIpv4)
	}
	return addrs
}

type VpnAddressSpaceError struct {
	Msg string
}

func (v *VpnAddressSpaceError) Error() string {
	return v.Msg
}

type ServerNotFound struct{}

func (s *ServerNotFound) Error() string { return "Server with the priority passed was not found." }

type cloudConfig struct {
	Image      string `json:"image"`
	Region     string `json:"region"`
	LinodeType string `json:"linode_type"`
}

/*
Log a message to the Contexts 'stream' io.Writer interface
*/
func (c *Configuration) Log(data ...string) {
	c.stream.Write([]byte(fmt.Sprintf(LogMsgTmpl, time.Now().String(), data)))

}

func (c *Configuration) SetConfigIO(impl DaemonConfigIO) {
	c.cfgIO = impl
}

func (c *Configuration) SetStreamIO(impl io.Writer) {
	c.stream = impl
}

/*
Calculate the VPN space details
*/
func (c *Configuration) CalculateVpnSpace() error {

	mask, _ := c.Service.VpnAddressSpace.Mask.Size()
	vpnNetwork := fmt.Sprintf("%s/%v", c.Service.VpnAddressSpace.IP.String(), mask)
	addresses, err := GetNetworkAddresses(vpnNetwork)
	if err != nil {
		return err
	}
	_, ntwrk, _ := net.ParseCIDR(vpnNetwork)
	addrSpace := map[string]bool{}
	for i := range addresses.Ipv4s {
		addrSpace[addresses.Ipv4s[i].String()] = false
	}
	usedAddresses := c.AllVpnAddresses()
	for i := range usedAddresses {
		c.Log("Checking: ", usedAddresses[i].String())
		c.Service.VpnAddresses[usedAddresses[i].String()] = true
	}
	c.Service.VpnAddresses = addrSpace
	c.Service.VpnAddressSpace = *ntwrk
	c.Service.VpnMask = addresses.Mask
	return nil
}

type ConfigurationBuilder struct {
	fileLocations []string
}

/*
Walk through all of the possible configuration avenues, and build out the configuration.
*/
func (c ConfigurationBuilder) Build() *Configuration { return nil }

func (c ConfigurationBuilder) readEnv() {}
func (c ConfigurationBuilder) readFiles() {

}

func (c ConfigurationBuilder) readServer() {}

type ConfigHostImpl struct {
	path string
}

func NewConfigHostImpl(path string) ConfigHostImpl {
	return ConfigHostImpl{path: path}
}

func (c ConfigHostImpl) Propogate(config *Configuration) {
	b, err := os.ReadFile(c.path)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(b, config)
	if err != nil {
		log.Fatal(err)
	}
	err = config.CalculateVpnSpace()
	if err != nil {
		log.Fatal(err)
	}

}

func (c ConfigHostImpl) Save(config Configuration) error {
	b, err := json.MarshalIndent(c, " ", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, b, 0666)

}

func NewConfigServerImpl(host string, port int, serverProto string) ConfigServerImpl {
	return ConfigServerImpl{
		http:  http.Client{},
		addr:  net.NS{Host: host},
		port:  port,
		proto: serverProto}
}

type ConfigServerImpl struct {
	http  http.Client
	addr  net.NS
	port  int
	proto string
}

func (s ConfigServerImpl) Propogate(config *Configuration) {
	resp, err := s.get("/get-config/" + string(config.Username))
	if err != nil {
		log.Fatal(err)
	}
	if err = json.Unmarshal(resp, &config); err != nil {
		log.Fatal(err)
	}
	err = config.CalculateVpnSpace()
	if err != nil {
		log.Fatal(err)
	}

}
func (s ConfigServerImpl) Save(config Configuration) error {
	_, err := s.post(config, "/update-config/"+string(config.Username))
	if err != nil {
		return err
	}
	return nil

}

/*
Agnostic GET call

	:param path: the path to attach to the HTTP server
*/
func (s ConfigServerImpl) get(path string) ([]byte, error) {
	url := fmt.Sprintf("%s://%s:%d%s", s.proto, s.addr.Host, s.port, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return b, nil

}

/*
Agnostic POST call

	    :param body: a struct that is JSON encodable
		:param path: the path to attach to the HTTP server
*/
func (s ConfigServerImpl) post(body interface{}, path string) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s://%s:%d%s", s.proto, s.addr.Host, s.port, path)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode > 299 {
		return nil, &ConfigError{}
	}
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return rb, nil

}

/*
Create a new Configuration struct with initialized maps
*/
func NewConfiguration(stream io.Writer, username Username) *Configuration {
	return &Configuration{Username: username, stream: stream, Service: serviceConfig{Servers: map[string]VpnServer{}, Clients: map[string]VpnClient{}, VpnAddresses: map[string]bool{}}}
}

func BlankConfig(path string) error {
	config := NewConfiguration(bytes.NewBuffer([]byte{}), "replace_me")
	b, err := json.Marshal(config)
	if err != nil {
		return err
	}
	os.WriteFile(path, b, 0666)
	return nil

}

type ConfigError struct {
	Msg string
}

func (c *ConfigError) Error() string {
	return "There was an error with the configuration: " + c.Msg
}

/*
###############################################################
########### section for the address space functions ###########
###############################################################
*/

type NetworkInterfaceNotFound struct{ Passed string }

// Implementing error interface
func (n *NetworkInterfaceNotFound) Error() string {
	return fmt.Sprintf("Interface: '%s' not found.", n.Passed)
}

type IpSubnetMapper struct {
	Ipv4s       []net.IP `json:"addresses"`
	NetworkAddr net.IP
	Current     net.IP
	Mask        int
}

/*
Get the next IPv4 address of the address specified in the 'addr' argument,

	:param addr: the address to get the next address of
*/
func getNextAddr(addr string) string {
	parsed, err := netip.ParseAddr(addr)
	if err != nil {
		log.Fatal("failed while parsing address in getNextAddr() ", err, "\n")
	}
	return parsed.Next().String()

}

/*
get the network address of the ip address in 'addr' with the subnet mask from 'cidr'

	    :param addr: the ipv4 address to get the network address of
		:param cidr: the CIDR notation of the subbet
*/
func getNetwork(addr string, cidr int) string {
	addr = fmt.Sprintf("%s/%v", addr, cidr)
	ip, net, err := net.ParseCIDR(addr)
	if err != nil {
		log.Fatal("failed whilst attempting to parse cidr in getNetwork() ", err, "\n")
	}
	return ip.Mask(net.Mask).String()

}

/*
Recursive function to get all of the IPv4 addresses for each IPv4 network that the host is on

	     :param ipmap: a pointer to an IpSubnetMapper struct which contains domain details such as
		               the subnet mask, the original network mask, and the current IP address used in the
					   recursive function
		:param max: This is safety feature to prevent stack overflows, so you can manually set the depth to
		            call the function
*/
func addressRecurse(ipmap *IpSubnetMapper) {

	next := getNextAddr(ipmap.Current.String())

	nextNet := getNetwork(next, ipmap.Mask)
	currentNet := ipmap.NetworkAddr.String()

	if nextNet != currentNet {
		return
	}
	ipmap.Current = net.ParseIP(next)

	ipmap.Ipv4s = append(ipmap.Ipv4s, net.ParseIP(next))
	addressRecurse(ipmap)
}

/*
Get all of the IPv4 addresses in the network that 'addr' belongs to. YOU MUST PASS THE ADDRESS WITH CIDR NOTATION
i.e. '192.168.50.1/24'

	:param addr: the ipv4 address to use for subnet discovery
*/
func GetNetworkAddresses(addr string) (*IpSubnetMapper, error) {
	ipmap := &IpSubnetMapper{Ipv4s: []net.IP{}}

	ip, net, err := net.ParseCIDR(addr)
	if err != nil {
		return nil, err
	}
	mask, err := strconv.Atoi(strings.Split(addr, "/")[1])
	if err != nil {
		return nil, err
	}
	ipmap.NetworkAddr = ip.Mask(net.Mask)
	ipmap.Mask = mask
	ipmap.Current = ip.Mask(net.Mask)
	addressRecurse(ipmap)

	return ipmap, nil

}

/*
##########################################
##### Initial daemon startup helpers #####
##########################################
*/

type StartupArgKeyname string

const (
	ConfigModeArg        StartupArgKeyname = "CONFIG_MODE"
	UsernameArg                            = "USERNAME"
	SecretsBackendKeyArg                   = "SECRET_BACKEND_KEY"
	ConfigServerAddr                       = "YOSAI_CONFIG_SERVER_ADDR"
	ConfigServerPort                       = "YOSAI_CONFIG_SERVER_PORT"
	ConfigServerProto                      = "YOSAI_CONFIG_SERVER_PROTO"
	ConfigFileLoc                          = "YOSAI_CONFIG_FILE_LOC"
)

type StartupRequirements struct {
	ConfigurationMode string
	Username          string
	SecretsBackendKey string
	ConfigServerPort  int
	ConfigServerAddr  string
	ConfigServerProto string
	ConfigFileLoc     string
}

/*
Grab user input for values passed via 'missing', and set the startup data configuration map with the value supplied

	    :param cfg: a map that holds the data required to start the daemon
		:param missing: a map of missing values that the function can use to index the cfg struct
*/
func getUserInput(cfg map[StartupArgKeyname]string, missing []StartupArgKeyname) {
	for i := range missing {
		fmt.Print(missing[i], ":")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		cfg[missing[i]] = strings.Trim(line, "\n")

	}
}

/*
Pass in a map of arguments supplied from program flags, and this function will attempt to try a
variety of different sources to fill in the gaps, i.e. environment vars, user input

	:param args: the argument map, likely called from the main function.
*/
func Turnkey(args map[StartupArgKeyname]string) StartupRequirements {
	cfg := map[StartupArgKeyname]string{
		ConfigModeArg:        "",
		UsernameArg:          "",
		SecretsBackendKeyArg: "",
		ConfigServerAddr:     "",
		ConfigServerPort:     "",
		ConfigServerProto:    "",
		ConfigFileLoc:        "",
	}
	fromEnv := map[StartupArgKeyname]string{
		ConfigModeArg:        os.Getenv(string(ConfigModeArg)),
		UsernameArg:          os.Getenv(string(UsernameArg)),
		SecretsBackendKeyArg: os.Getenv(string(SecretsBackendKeyArg)),
		ConfigServerAddr:     os.Getenv(string(ConfigServerAddr)),
		ConfigServerProto:    os.Getenv(string(ConfigServerProto)),
		ConfigServerPort:     os.Getenv(string(ConfigServerPort)),
		ConfigFileLoc:        os.Getenv(string(ConfigFileLoc))}
	for i := range args {
		if args[i] == "" {
			continue
		}
		cfg[i] = args[i]
	}
	for i := range fromEnv {
		if fromEnv[i] == "" {
			continue
		}
		if cfg[i] != "" {
			fmt.Println("Overwriting value: ", i, " from set environment variable.")
		}
		cfg[i] = fromEnv[i]
	}
	var missingValues []StartupArgKeyname
	for i := range cfg {
		if cfg[i] == "" {
			fmt.Println("missing value for: ", i)
			missingValues = append(missingValues, i)
		}
	}
	if len(missingValues) != 0 {
		getUserInput(cfg, missingValues)
	}
	portInt, err := strconv.Atoi(cfg[ConfigServerPort])
	if err != nil {
		fmt.Println("A non-parseable value: '", cfg[ConfigServerPort], "' was passed in. Please check your inputs. Defaulting to 8080")
		portInt = 8080
	}
	if portInt < 0 || portInt > 65535 {
		fmt.Println("An unuseable port was passed: '", portInt, "', defaultint to 8080")
		portInt = 8080
	}

	return StartupRequirements{
		ConfigurationMode: cfg[ConfigModeArg],
		Username:          cfg[UsernameArg],
		SecretsBackendKey: cfg[SecretsBackendKeyArg],
		ConfigServerAddr:  cfg[ConfigServerAddr],
		ConfigServerProto: cfg[ConfigServerProto],
		ConfigServerPort:  portInt,
		ConfigFileLoc:     cfg[ConfigFileLoc],
	}
}
