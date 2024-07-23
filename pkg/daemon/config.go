package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

var EnvironmentVariables = []string{
	"HASHICORP_VAULT_URL",
	"HASHICORP_VAULT_KEY",
	"SEMAPHORE_SERVER_URL",
}

const DefaultConfigLoc = "./.config.json"

type Configuration interface {
	SetServerName(val string)
	SetRepo(val string)
	SetBranch(val string)
	SetPlaybookName(val string)
	SetImage(val string)
	SetRegion(val string)
	SetLinodeType(val string)
	SetVpnServer(val string)
	SetVpnServerId(val int)
	SetVpnNetwork(val string) error
	VpnClientIpAddr() string
	VpnServerIpAddr() string
	VpnServerPort() int
	VpnServerNetwork() string
	VpnServerId() int
	VpnServer() string
	ServerName() string
	Repo() string
	Branch() string
	PlaybookName() string
	Image() string
	Region() string
	LinodeType() string
	ConfigRouter(msg SockMessage) SockMessage
	Save(path string) error
}

type ConfigurationActionOut struct {
	Config string
}

// Implementing the ActionOut interface
func (c ConfigurationActionOut) GetResult() string {
	return c.Config

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

// Implemeting the interface to make this callable via the CLI
func (c *ConfigFromFile) ConfigRouter(msg SockMessage) SockMessage {
	switch msg.Method {
	case "show":

		b, err := json.MarshalIndent(&c, "", "   ")
		if err != nil {
			return *NewSockMessage(MsgResponse, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, b)
	default:
		return *NewSockMessage(MsgResponse, []byte("Unresolved Method"))
	}
}

type ConfigFromFile struct {
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
	VpnServer        string `json:"vpn_server"`
	VpnServerId      int    `json:"vpn_server_id"`
	VpnServerName    string `json:"vpn_server_name"`
	VpnServerNetwork string `json:"vpn_server_network"`
	VpnServerIPv4    string `json:"vpn_server_ipv4"`
	VpnServerPort    int    `json:"vpn_server_port"`
	VpnClientIPv4    string `json:"vpn_client_ipv4"`
}

func (c *ConfigFromFile) SetRepo(val string)         { c.Ansible.Repo = val }
func (c *ConfigFromFile) SetBranch(val string)       { c.Ansible.Branch = val }
func (c *ConfigFromFile) SetPlaybookName(val string) { c.Ansible.PlaybookName = val }
func (c *ConfigFromFile) SetImage(val string)        { c.Cloud.Image = val }
func (c *ConfigFromFile) SetRegion(val string)       { c.Cloud.Region = val }
func (c *ConfigFromFile) SetLinodeType(val string)   { c.Cloud.LinodeType = val }
func (c *ConfigFromFile) SetVpnServer(val string)    { c.Service.VpnServer = val }
func (c *ConfigFromFile) SetVpnServerId(val int)     { c.Service.VpnServerId = val }
func (c *ConfigFromFile) SetServerName(val string)   { c.Service.VpnServerName = val }
func (c *ConfigFromFile) SetVpnNetwork(val string) error {
	addr, ntwrk, err := net.ParseCIDR(val)
	if err != nil {
		return err
	}
	ntwrkSp := strings.Split(ntwrk.String(), "/")
	cidr := ntwrkSp[1]
	parsed, _ := netip.ParseAddr(addr.String())
	clientIp := parsed.Next()
	serverIp := clientIp.Next()

	c.Service.VpnServerNetwork = ntwrk.String()
	c.Service.VpnServerIPv4 = serverIp.String() + "/" + cidr
	c.Service.VpnClientIPv4 = clientIp.String() + "/" + cidr
	return nil
}

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
func (c *ConfigFromFile) VpnServerId() int {
	return c.Service.VpnServerId
}

func (c *ConfigFromFile) VpnServer() string {
	return c.Service.VpnServer
}

func (c *ConfigFromFile) ServerName() string {
	return c.Service.VpnServerName
}
func (c *ConfigFromFile) VpnServerIpAddr() string {
	return c.Service.VpnServerIPv4
}
func (c *ConfigFromFile) VpnClientIpAddr() string {
	return c.Service.VpnClientIPv4
}
func (c *ConfigFromFile) VpnServerNetwork() string {
	return c.Service.VpnServerNetwork
}
func (c *ConfigFromFile) VpnServerPort() int {
	return c.Service.VpnServerPort
}

func ReadConfig(path string) Configuration {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var config ConfigFromFile
	err = json.Unmarshal(b, &config)
	if err != nil {
		log.Fatal(err)
	}
	return &config

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
	}
	b, err := json.Marshal(config)
	if err != nil {
		return err
	}
	os.WriteFile(path, b, 0666)
	return nil

}
