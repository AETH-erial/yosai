package hashicorp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
)

const (
	SecretsApiPath = "v1/secret/data"
)

type VaultAdd struct {
	Data map[string]string `json:"data"`
}

type VaultResponse struct {
	Data VaultResponseInner `json:"data"`
}

type VaultResponseInner struct {
	Data VaultItem `json:"data"`
}

type VaultItem struct {
	Public string `json:"public"`
	Secret string `json:"secret"`
	Type   string `json:"type"`
}

type VaultConnection struct {
	VaultUrl  string
	HttpProto string
	KeyRing   daemon.DaemonKeyRing
	Client    *http.Client
}

// Returns the 'public' field of the credential, i.e. a username or something
func (v VaultResponse) GetPublic() string {
	return v.Data.Data.Public
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
Retrieve a key from hashicorp. the 2nd argument, 'name' is the path of the secret in hashicorp

	:param keyring: a daemon.DaemonKeyRing interface that will have the hashicorp API key
	:param name: the name of the secret in hashicorp. It will be injected as the 'path' in the API call,
	See the Hashicorp Vault documentation for details
*/
func (v VaultConnection) GetKey(name string) (daemon.Key, error) {
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
		return vaultResp, errors.New(resp.Status)
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
func (v VaultConnection) AddKey(name string, key daemon.Key) error {
	body := VaultAdd{
		Data: map[string]string{key.GetPublic(): key.GetSecret()},
	}
	b, err := json.Marshal(&body)
	if err != nil {
		return err
	}
	vaultBase := fmt.Sprintf("%s://%s/%s/%s", v.HttpProto, v.VaultUrl, SecretsApiPath, key.GetPublic())
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
