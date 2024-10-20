package keytags

type Keytagger interface {
	HashicorpVaultKeyname() string    // returns the API/Vault key's name
	LinodeApiKeyname() string         // Returns the Linode API key's name
	VpsRootKeyname() string           // Returns the VPS Root user credentials name
	VpsSvcAccKeyname() string         // Returns the VPS service account credentials name
	SystemSshKeyname() string         // get the ssh key name for the system
	SemaphoreApiKeyname() string      // Returns the Semaphore API key name
	WgKeypairKeyname() string         // returns the keyname of the Wireguard server keypair
	AllKeys() []string                // Returns all of the key names
	GetAnsibleKeys() []string         // Returns all the keynames that need to be added to Semaphore
	ProtectedKeys() map[string]string // Get protected keys that shall not be deleted and reloaded when the keyring is synced with the backend
}

type ConstKeytag struct {
}

func (c ConstKeytag) HashicorpVaultKeyname() string  { return HASHICORP_VAULT_KEYNAME }
func (c ConstKeytag) LinodeApiKeyname() string       { return LINODE_API_KEYNAME }
func (c ConstKeytag) VpsRootKeyname() string         { return VPS_ROOT_PASS_KEYNAME }
func (c ConstKeytag) VpsSvcAccKeyname() string       { return VPS_SUDO_USER_KEYNAME }
func (c ConstKeytag) VpsSvcAccSshKeyname() string    { return VPS_SSH_KEY_KEYNAME }
func (c ConstKeytag) SemaphoreApiKeyname() string    { return SEMAPHORE_API_KEYNAME }
func (c ConstKeytag) GitSshKeyname() string          { return GIT_SSH_KEYNAME }
func (c ConstKeytag) VpsSvcAccSshPubkeySeed() string { return VPS_PUBKEY_SEED_KEYNAME }
func (c ConstKeytag) WgKeypairKeyname() string       { return WG_KEYPAIR_KEYNAME }
func (c ConstKeytag) SystemSshKeyname() string       { return SYSTEM_SSH_KEYNAME }
func (c ConstKeytag) GetAnsibleKeys() []string {
	return []string{

		SYSTEM_SSH_KEYNAME,
		VPS_ROOT_PASS_KEYNAME,
		VPS_SUDO_USER_KEYNAME,
	}
}
func (c ConstKeytag) AllKeys() []string {
	return []string{
		c.HashicorpVaultKeyname(),
		c.LinodeApiKeyname(),
		c.VpsRootKeyname(),
		c.VpsSvcAccKeyname(),
		c.SystemSshKeyname(),
		c.SemaphoreApiKeyname(),
		//		c.WgKeypairKeyname(),
	}
}
func (c ConstKeytag) ProtectedKeys() map[string]string {
	return map[string]string{
		c.HashicorpVaultKeyname(): "",
	}
}

type ConfigFileKeytag struct {
	HashicorpVaultKn string `json:"hashicorp_vault_keyname"`
	LinodeApiKn      string `json:"linode_api_keyname"`
	VpsRootKn        string `json:"vps_root_keyname"`
	VpsSvcAccKn      string `json:"vps_svc_acc_keyname"`
	VpsSvcAccSshKn   string `json:"vps_svc_ssh_keyname"`
	SemaphoreApiKn   string `json:"semaphore_api_keyname"`
	GitSshKn         string `json:"git_ssh_keyname"`
}

func (c ConfigFileKeytag) HashicorpVaultKeyname() string { return c.HashicorpVaultKn }
func (c ConfigFileKeytag) LinodeApiKeyname() string      { return c.LinodeApiKn }
func (c ConfigFileKeytag) VpsRootKeyname() string        { return c.VpsRootKn }
func (c ConfigFileKeytag) VpsSvcAccKeyname() string      { return c.VpsSvcAccKn }
func (c ConfigFileKeytag) VpsSvcAccSshKeyname() string   { return c.VpsSvcAccSshKn }
func (c ConfigFileKeytag) SemaphoreApiKeyname() string   { return c.SemaphoreApiKn }
func (c ConfigFileKeytag) GitSshKeyname() string         { return c.GitSshKn }
func (c ConfigFileKeytag) GetAnsibleKeys() []string {
	return []string{
		c.GitSshKn,
		c.VpsRootKn,
		c.VpsSvcAccKn,
		c.VpsSvcAccSshKn,
	}
}
func (c ConfigFileKeytag) AllKeys() []string {
	return []string{
		c.HashicorpVaultKeyname(),
		c.LinodeApiKeyname(),
		c.VpsRootKeyname(),
		c.VpsSvcAccKeyname(),
		c.VpsSvcAccSshKeyname(),
		c.SemaphoreApiKeyname(),
		c.GitSshKeyname(),
	}
}

const HASHICORP_VAULT_KEYNAME = "HASHICORP_VAULT_KEY"
const LINODE_API_KEYNAME = "LINODE_API_KEY"
const VPS_ROOT_PASS_KEYNAME = "VPS_ROOT_USER"
const VPS_SUDO_USER_KEYNAME = "VPS_SUDO_USER"
const VPS_SSH_KEY_KEYNAME = "VPS_SSH_KEY"
const SEMAPHORE_API_KEYNAME = "SEMAPHORE_API_KEY"
const GIT_SSH_KEYNAME = "GIT_SSH_KEY"
const VPS_PUBKEY_SEED_KEYNAME = "VPS_PUBKEY_SEED"
const WG_KEYPAIR_KEYNAME = "WG_KEYPAIR"
const SYSTEM_SSH_KEYNAME = "SYSTEM_SSH_KEYNAME"
