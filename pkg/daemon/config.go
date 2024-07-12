package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strconv"
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
	ConfigRouter(arg ActionIn) (ActionOut, error)
	Save(path string) error
}

type ConfigurationActionOut struct {
	Config string
}

// Implementing the ActionOut interface
func (c ConfigurationActionOut) GetResult() string {
	return c.Config

}

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
func (c *ConfigFromFile) ConfigRouter(arg ActionIn) (ActionOut, error) {
	var out ConfigurationActionOut
	switch arg.Method() {
	case "show":
		return ConfigurationActionOut{Config: c.VpnServer()}, nil
	case "all":

		var out ConfigurationActionOut
		b, err := json.MarshalIndent(&c, "", "   ")
		if err != nil {
			return out, err
		}
		out = ConfigurationActionOut{Config: string(b)}
		return out, nil
	case "set":
		kv := strings.Split(arg.Arg(), "=")
		if len(kv) < 2 {
			return out, &InvalidAction{Msg: "Please pass configuration in the form of 'val'='key'"}
		}
		k := kv[0]
		v := kv[1]
		switch k {
		case "repo_url":
			c.SetRepo(v)
			return ConfigurationActionOut{Config: "ansible.repo_url set to: " + v}, nil
		case "branch":
			c.SetBranch(v)
			return ConfigurationActionOut{Config: "ansible.branch set to: " + v}, nil
		case "playbook_name":
			c.SetPlaybookName(v)
			return ConfigurationActionOut{Config: "ansible.playbook_name set to: " + v}, nil
		case "image":
			c.SetImage(v)
			return ConfigurationActionOut{Config: "cloud.image set to: " + v}, nil
		case "region":
			c.SetRegion(v)
			return ConfigurationActionOut{Config: "cloud.region set to: " + v}, nil
		case "linode_type":
			c.SetLinodeType(v)
			return ConfigurationActionOut{Config: "cloud.linode_type set to: " + v}, nil
		case "vpn_server":
			err := net.ParseIP(v)
			if err == nil { // because a nil return equates to an invalid IP
				return out, &InvalidAction{Msg: "Passed address: " + v + " is not a valid IPv4."}
			}
			c.SetVpnServer(v)
			return ConfigurationActionOut{Config: "service.vpn_server set to: " + v}, nil
		case "vpn_server_id":
			vint, err := strconv.Atoi(v)
			if err != nil {
				return out, &InvalidAction{Msg: "Invalid VPN server ID. Must be an int"}
			}
			c.SetVpnServerId(int(vint))
			return ConfigurationActionOut{Config: "service.vpn_server_id set to: " + v}, nil
		case "server_name":
			c.SetServerName(v)
			return ConfigurationActionOut{Config: "service.server_name set to: " + v}, nil
		case "vpn_network":
			err := c.SetVpnNetwork(v)
			if err != nil {
				return out, &InvalidAction{Msg: "Couldnt parse the passed address: " + v}
			}
			return ConfigurationActionOut{Config: fmt.Sprintf("service.vpn_network: %s\nservice.vpn_client_ipv4: %s\nservice.vpn_server_ipv4: %s", c.VpnServerNetwork(), c.VpnClientIpAddr(), c.VpnServerIpAddr())}, nil
		}
	case "save":
		err := c.Save(DefaultConfigLoc)
		if err != nil {
			return out, err

		}
		return ConfigurationActionOut{Config: "Configuration was saved to disk."}, nil
	}
	return out, &InvalidAction{Msg: "unresolved action was passed.", Action: arg.Method()}
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
