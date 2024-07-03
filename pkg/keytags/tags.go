package keytags

type Keytagger interface {
	HashicorpVaultKeyname() string // returns the API/Vault key's name
	LinodeApiKeyname() string      // Returns the Linode API key's name
	VpsRootKeyname() string        // Returns the VPS Root user credentials name
	VpsSvcAccKeyname() string      // Returns the VPS service account credentials name
	VpsSvcAccSshKeyname() string   // returns the VPS service account's SSH key name
	SemaphoreApiKeyname() string   // Returns the Semaphore API key name
	GitSshKeyname() string         // Returns the name of the SSH key used to pull from the git server
}

// TODO: implement the Keytagger interface
type ConstKeytag struct {
}

// TODO: implement the Keytagger interface
type ConfigFileKeytag struct {
}

const HASHICORP_VAULT_KEYNAME = "HASHICORP_VAULT_KEY"
const LINODE_API_KEYNAME = "LINODE_API_KEY"
const VPS_ROOT_PASS_KEYNAME = "VPS_ROOT_USER"
const VPS_SUDO_USER_KEYNAME = "VPS_SUDO_USER"
const VPS_SSH_KEY_KEYNAME = "VPS_SSH_KEY"
const SEMAPHORE_API_KEYNAME = "SEMAPHORE_API_KEY"
const GIT_SSH_KEYNAME = "GIT_SSH_KEY"

var AllTags map[string]struct{} = map[string]struct{}{
	HASHICORP_VAULT_KEYNAME: struct{}{},
	LINODE_API_KEYNAME:      struct{}{},
	VPS_ROOT_PASS_KEYNAME:   struct{}{},
	VPS_SUDO_USER_KEYNAME:   struct{}{},
	VPS_SSH_KEY_KEYNAME:     struct{}{},
	SEMAPHORE_API_KEYNAME:   struct{}{},
	GIT_SSH_KEYNAME:         struct{}{},
}
