package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const LogMsgTmpl = "YOSAI Daemon ||| time: %s ||| %s\n"

var EnvironmentVariables = []string{
	"HASHICORP_VAULT_URL",
	"HASHICORP_VAULT_KEY",
	"SEMAPHORE_SERVER_URL",
}

const DefaultConfigLoc = "./.config.json"

type Configuration interface {
	SetRepo(val string)
	SetBranch(val string)
	SetPlaybookName(val string)
	SetImage(val string)
	SetRegion(val string)
	SetLinodeType(val string)
	SetSecretsBackend(val string)
	SetSecretsBackendUrl(val string)
	GetServer(priority int8) (VpnServer, error)
	SecretsBackend() string
	SecretsBackendUrl() string
	Repo() string
	Branch() string
	PlaybookName() string
	Image() string
	Region() string
	LinodeType() string
	Log(data ...string)
	ConfigRouter(msg SockMessage) SockMessage
	Save(path string) error
}

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

// Router for all peer related functions
func (c *ConfigFromFile) PeerRouter(msg SockMessage) SockMessage {
	switch msg.Method {
	case "add":
		var peer VpnClient
		err := json.Unmarshal(msg.Body, &peer)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		addr, err := c.GetAvailableVpnIpv4()
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Client: "+c.AddClient(addr, peer.Pubkey, peer.Name)+" Successfully added."))
	case "delete":
		var req VpnClient
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		peer, err := c.GetClient(req.Name)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}

		delete(c.Service.Clients, peer.Name)
		err = c.FreeAddress(peer.VpnIpv4.String())
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Client: "+peer.Name+" Successfully deleted from the config."))

	default:
		return *NewSockMessage(MsgResponse, REQUEST_UNRESOLVED, []byte("Unresolved method: "+msg.Method))
	}

}

// Router for all server related functions
func (c *ConfigFromFile) ServerRouter(msg SockMessage) SockMessage {
	var req VpnServer
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	switch msg.Method {
	case "add":
		addr, err := c.GetAvailableVpnIpv4()
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		name := c.AddServer(addr, req.Name, req.WanIpv4, req.Port)
		c.Log("address: ", addr.String(), "name:", name)
		return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Server: "+name+" Successfully added."))
	case "delete":
		server, err := c.GetServer(req.Name)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}

		delete(c.Service.Servers, server.Name)
		err = c.FreeAddress(server.VpnIpv4.String())
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Server: "+server.Name+" Successfully deleted from the config."))

	default:
		return *NewSockMessage(MsgResponse, REQUEST_UNRESOLVED, []byte("Unresolved method: "+msg.Method))
	}

}

// Implemeting the interface to make this callable via the CLI
func (c *ConfigFromFile) ConfigRouter(msg SockMessage) SockMessage {
	switch msg.Method {
	case "show":
		b, err := json.MarshalIndent(&c, "", "   ")
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, b)
	case "save":
		err := c.Save(DefaultConfigLoc)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Configuration saved successfully."))
	case "reload":
		b, err := os.ReadFile(DefaultConfigLoc)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		err = json.Unmarshal(b, c)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Configuration reloaded successfully."))
	default:
		return *NewSockMessage(MsgResponse, REQUEST_UNRESOLVED, []byte("Unresolved Method"))
	}
}

type ConfigFromFile struct {
	stream  io.Writer
	Cloud   cloudConfig   `json:"cloud"`
	Ansible ansibleConfig `json:"ansible"`
	Service serviceConfig `json:"service"`
}

type ansibleConfig struct {
	Repo         string `json:"repo_url"`
	Branch       string `json:"branch"`
	PlaybookName string `json:"playbook_name"`
}

type serviceConfig struct {
	Servers           map[string]VpnServer
	Clients           map[string]VpnClient
	VpnAddressSpace   net.IPNet
	VpnAddresses      map[string]bool // Each key is a IPv4 in the VPN, and its corresponding value is what denotes if its in use or not. False == 'In use', True == 'available'
	VpnMask           int             // The mask of the VPN
	VpnServerPort     int             `json:"vpn_server_port"`
	SecretsBackend    string          `json:"secrets_backend"`
	SecretsBackendUrl string          `json:"secrets_backend_url"`
}

func (c *ConfigFromFile) GetServer(name string) (VpnServer, error) {
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

func (c *ConfigFromFile) GetClient(name string) (VpnClient, error) {
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
func (c *ConfigFromFile) AddServer(addr net.IP, name string, wan string, port int) string {
	server, ok := c.Service.Servers[name]
	var serverLabel string
	if ok {
		serverLabel = c.resolveName(server.Name, name)
	} else {
		serverLabel = name
	}
	c.Service.Servers[serverLabel] = VpnServer{Name: serverLabel, WanIpv4: wan, VpnIpv4: addr, Port: port}
	return serverLabel

}

type VpnClient struct {
	Name    string `json:"name"`
	VpnIpv4 net.IP
	Pubkey  string `json:"pubkey"`
}

type VpnServer struct {
	Name    string `json:"name"`     // this Label is what is used to index that server and its data within the Daemons model of the VPN environment
	WanIpv4 string `json:"wan_ipv4"` // Public IPv4
	VpnIpv4 net.IP // the IP address that the server will occupy on the network
	Port    int
}

/*
Retrieve an available IPv4 from the VPN's set address space. Returns an error if an internal address cant be
parsed to a valid IPv4, or if there are no available addresses left.
*/
func (c *ConfigFromFile) GetAvailableVpnIpv4() (net.IP, error) {
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
func (c *ConfigFromFile) VpnClients() []VpnClient {
	clients := []VpnClient{}
	for _, val := range c.Service.Clients {
		clients = append(clients, val)
	}
	return clients
}

/*
resolve naming collision in the client list

	:param existingName: the name of the existing client in the client list
	:param dupeName: the desired name of the client to be added
*/
func (c *ConfigFromFile) resolveName(existingName string, dupeName string) string {
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
func (c *ConfigFromFile) AddClient(addr net.IP, pubkey string, name string) string {
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
func (c *ConfigFromFile) FreeAddress(addr string) error {
	_, ok := c.Service.VpnAddresses[addr]
	if !ok {
		return &VpnAddressSpaceError{Msg: "Address: " + addr + " is not in the designated VPN Address space."}
	}
	c.Service.VpnAddresses[addr] = false
	return nil
}

type VpnAddressSpaceError struct {
	Msg string
}

func (v *VpnAddressSpaceError) Error() string {
	return v.Msg
}

type ServerNotFound struct{}

func (s *ServerNotFound) Error() string { return "Server with the priority passed was not found." }

func (c *ConfigFromFile) SetRepo(val string)              { c.Ansible.Repo = val }
func (c *ConfigFromFile) SetBranch(val string)            { c.Ansible.Branch = val }
func (c *ConfigFromFile) SetPlaybookName(val string)      { c.Ansible.PlaybookName = val }
func (c *ConfigFromFile) SetImage(val string)             { c.Cloud.Image = val }
func (c *ConfigFromFile) SetRegion(val string)            { c.Cloud.Region = val }
func (c *ConfigFromFile) SetLinodeType(val string)        { c.Cloud.LinodeType = val }
func (c *ConfigFromFile) SetSecretsBackend(val string)    { c.Service.SecretsBackend = val }
func (c *ConfigFromFile) SetSecretsBackendUrl(val string) { c.Service.SecretsBackendUrl = val }

func (c *ConfigFromFile) Repo() string {
	return c.Ansible.Repo
}

func (c *ConfigFromFile) Branch() string {
	return c.Ansible.Branch
}

func (c *ConfigFromFile) PlaybookName() string { return c.Ansible.PlaybookName }

type cloudConfig struct {
	Image      string `json:"image"`
	Region     string `json:"region"`
	LinodeType string `json:"linode_type"`
}

func (c *ConfigFromFile) Image() string {
	return c.Cloud.Image
}

func (c *ConfigFromFile) Region() string {
	return c.Cloud.Region
}

func (c *ConfigFromFile) LinodeType() string {
	return c.Cloud.LinodeType
}

func (c *ConfigFromFile) VpnServerPort() int {
	return c.Service.VpnServerPort
}
func (c *ConfigFromFile) SecretsBackend() string {
	return c.Service.SecretsBackend
}
func (c *ConfigFromFile) SecretsBackendUrl() string {
	return c.Service.SecretsBackendUrl
}

/*
Log a message to the Contexts 'stream' io.Writer interface
*/
func (c *ConfigFromFile) Log(data ...string) {
	c.stream.Write([]byte(fmt.Sprintf(LogMsgTmpl, time.Now().String(), data)))

}

func ReadConfig(path string) *ConfigFromFile {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	config := &ConfigFromFile{
		stream: os.Stdout,
		Service: serviceConfig{
			Clients: map[string]VpnClient{},
			Servers: map[string]VpnServer{},
		},
	}
	err = json.Unmarshal(b, config)
	if err != nil {
		log.Fatal(err)
	}
	mask, _ := config.Service.VpnAddressSpace.Mask.Size()
	vpnNetwork := fmt.Sprintf("%s/%v", config.Service.VpnAddressSpace.IP.String(), mask)
	addresses, err := GetNetworkAddresses(vpnNetwork)
	if err != nil {
		log.Fatal(err)
	}

	_, ntwrk, _ := net.ParseCIDR(vpnNetwork)
	if config.Service.VpnAddresses == nil {
		addrSpace := map[string]bool{}
		for i := range addresses.Ipv4s {
			addrSpace[addresses.Ipv4s[i].String()] = false
		}
		config.Service.VpnAddresses = addrSpace
	}
	config.Service.VpnAddressSpace = *ntwrk
	config.Service.VpnMask = addresses.Mask
	return config

}

func (c *ConfigFromFile) Save(path string) error {
	b, err := json.MarshalIndent(c, " ", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0666)

}

func BlankConfig(path string) error {
	config := ConfigFromFile{
		Cloud: cloudConfig{
			Image:      "",
			Region:     "",
			LinodeType: "",
		},
		Ansible: ansibleConfig{
			Repo:   "",
			Branch: "",
		},
		Service: serviceConfig{},
	}
	b, err := json.Marshal(config)
	if err != nil {
		return err
	}
	os.WriteFile(path, b, 0666)
	return nil

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
