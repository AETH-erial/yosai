package hashicorp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/keyring"
)

const (
	SecretsApiPath = "v1/kv/data"
)

type VaultAdd struct {
	Data VaultItem `json:"data"`
}

type VaultResponse struct {
	Data VaultResponseInner `json:"data"`
}

type VaultResponseInner struct {
	Data VaultItem `json:"data"`
}

type VaultItem struct {
	Name     string          `json:"name"`
	Username config.Username `json:"username"`
	Public   string          `json:"public"`
	Secret   string          `json:"secret"`
	Type     keyring.KeyType `json:"type"`
}

type VaultConnection struct {
	VaultUrl  string
	HttpProto string
	KeyRing   keyring.DaemonKeyRing
	Client    *http.Client
}

func (v VaultItem) GetPublic() string        { return v.Public }
func (v VaultItem) GetSecret() string        { return v.Secret }
func (v VaultItem) GetType() keyring.KeyType { return v.Type }
func (v VaultItem) Prepare() string {
	return "Unimplemented method"
}
func (v VaultItem) Owner() config.Username {
	return v.Username
}

// Returns the 'public' field of the credential, i.e. a username or something
func (v VaultResponse) GetPublic() string {
	return v.Data.Data.Public
}

func (v VaultResponse) Owner() config.Username {
	return v.Data.Data.Username
}

// returns the 'private' field of the credential, like the API key or password
func (v VaultResponse) GetSecret() string {
	return v.Data.Data.Secret
}

// this is an extra implementation so VaultResponse can implement the daemon.Key interface
func (v VaultResponse) Prepare() string {
	if v.Data.Data.Type == "bearer" {
		return fmt.Sprintf("Bearer %s", v.GetSecret())
	}
	if v.Data.Data.Type == "basic" {

		encodedcreds := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", v.GetPublic(), v.GetSecret())))
		return fmt.Sprintf("Basic %s", encodedcreds)
	}
	return "CREDENTIAL TYPE INVALID"
}

/*
Implementing the daemon.Key interface and returning the keys 'type'
*/
func (v VaultResponse) GetType() keyring.KeyType {
	return v.Data.Data.Type
}

/*
Retrieve a key from hashicorp. the 2nd argument, 'name' is the path of the secret in hashicorp

	:param keyring: a daemon.DaemonKeyRing interface that will have the hashicorp API key
	:param name: the name of the secret in hashicorp. It will be injected as the 'path' in the API call,
	See the Hashicorp Vault documentation for details
*/
func (v VaultConnection) GetKey(name string) (keyring.Key, error) {
	vaultBase := fmt.Sprintf("%s://%s/%s/%s", v.HttpProto, v.VaultUrl, SecretsApiPath, name)
	var vaultResp VaultResponse
	req, err := http.NewRequest("GET", vaultBase, nil)
	if err != nil {
		return vaultResp, err
	}
	req.Header.Add("Content-Type", "application/json")
	vaultApiKey, err := v.KeyRing.GetKey(keytags.HASHICORP_VAULT_KEYNAME)
	if err != nil {
		return vaultResp, err
	}
	req.Header.Add("Authorization", vaultApiKey.Prepare())
	resp, err := v.Client.Do(req)
	if err != nil {
		return vaultResp, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return vaultResp, keyring.KeyNotFound
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return vaultResp, err
	}
	err = json.Unmarshal(b, &vaultResp)
	if err != nil {
		return vaultResp, err
	}
	return vaultResp, nil
}

/*
Add the root users for your VPS to Hashicorp vault

	:param pass: the password to store in vault
*/
func (v VaultConnection) AddKey(name string, key keyring.Key) error {
	body := VaultAdd{
		Data: VaultItem{Public: key.GetPublic(), Secret: key.GetSecret(), Type: key.GetType()},
	}
	b, err := json.Marshal(&body)
	if err != nil {
		return err
	}
	vaultBase := fmt.Sprintf("%s://%s/%s/%s", v.HttpProto, v.VaultUrl, SecretsApiPath, name)
	req, err := http.NewRequest("POST", vaultBase, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	vaultApiKey, err := v.KeyRing.GetKey(keytags.HASHICORP_VAULT_KEYNAME)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", vaultApiKey.Prepare())
	resp, err := v.Client.Do(req)
	if err != nil {
		return err

	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &HashicorpClientError{Msg: resp.Status}
	}
	return nil
}

/*
Removes a key from the vault

	:param name: the 'path' of the key as Hashicorp knows it
*/
func (v VaultConnection) RemoveKey(name string) error {

	vaultBase := fmt.Sprintf("%s://%s/%s/%s", v.HttpProto, v.VaultUrl, SecretsApiPath, name)
	req, err := http.NewRequest("DELETE", vaultBase, nil)
	if err != nil {
		return err
	}
	vaultApiKey, err := v.KeyRing.GetKey(keytags.HASHICORP_VAULT_KEYNAME)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", vaultApiKey.Prepare())
	req.Header.Add("Content-Type", "application/json")

	resp, err := v.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &HashicorpClientError{Msg: resp.Status}
	}
	return nil
}

// Return the resource name for logging purposes
func (v VaultConnection) Source() string {
	return "Hashicorp Vault"
}

/*
Handles the routing for the hashicorp keyring routes

	:param msg: a daemon.SockMessage that contains request data
*/
func (v VaultConnection) VaultRouter(msg daemonproto.SockMessage) daemonproto.SockMessage {
	switch msg.Method {
	case "add":
		var req VaultItem
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
		}
		err = v.AddKey(req.Name, req)
		if err != nil {
			return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
		}
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Key successfully added."))
	default:
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_UNRESOLVED, []byte("Unresolvable method"))

	}
}

/*
#####################
###### ERRORS #######
#####################
*/
type HashicorpClientError struct {
	Msg string
}

func (h *HashicorpClientError) Error() string {
	return fmt.Sprintf("There was an error with the client call: %s", h.Msg)
}
