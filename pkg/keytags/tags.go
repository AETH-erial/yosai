package keytags

const HASHICORP_VAULT_KEYNAME = "HASHICORP_VAULT_KEY"
const LINODE_API_KEYNAME = "LINODE_API_KEY"
const SERVER_ROOT_PASS_KEYNAME = "ROOT_USER"
const SERVER_SSH_KEY_KEYNAME = "ROOT_SSHKEY"
const SEMAPHORE_API_KEYNAME = "SEMAPHORE_API_KEY"
const GIT_SSH_KEYNAME = "GIT_SSH_KEY"

var AllTags map[string]struct{} = map[string]struct{}{
	HASHICORP_VAULT_KEYNAME:  struct{}{},
	LINODE_API_KEYNAME:       struct{}{},
	SERVER_ROOT_PASS_KEYNAME: struct{}{},
	SERVER_SSH_KEY_KEYNAME:   struct{}{},
	SEMAPHORE_API_KEYNAME:    struct{}{},
	GIT_SSH_KEYNAME:          struct{}{},
}
