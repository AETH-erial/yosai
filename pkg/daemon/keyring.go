package daemon

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

type KeyGetterActionOut struct {
	Private string
	Public  string
}

func (k KeyGetterActionOut) GetResult() string {
	return fmt.Sprintf("Public: %s\nPrivate: %s\n", k.Public, k.Private)
}

type VpsRootUser struct {
	Password string
	Pubkey   string
}

type BearerAuth struct {
	Secret string // Likely would be the API key for the API
}

/*
Format a bearer auth payload according to RFC 6750
*/
func (b BearerAuth) Prepare() string {
	return fmt.Sprintf("Bearer %s", b.Secret)
}

/*
Return the 'public' identifier, which for bearer auth the closest is going to be the auth type
*/
func (b BearerAuth) GetPublic() string {
	return "Bearer"
}

/*
Return the private data for this auth type
*/
func (b BearerAuth) GetSecret() string {
	return b.Secret
}

/*
Spit out a string with the data so that we can implement the 'ActionOut' interface
*/
func (b BearerAuth) GetResult() string {
	return fmt.Sprintf("Public: %s\nSecret: %s\n", b.GetPublic(), b.GetSecret())
}

type BasicAuth struct {
	Username string // Username for basic auth
	Password string // subsequent password for basic auth
}

/*
Encode a basic auth payload according to RFC 7617
*/
func (b BasicAuth) Prepare() string {
	encodedcreds := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", b.Username, b.Password)))
	return fmt.Sprintf("Basic %s", encodedcreds)
}

/*
Return the 'public' identifier, which for basic auth it will return the username
*/
func (b BasicAuth) GetPublic() string {
	return b.Password
}

/*
Return the private data for this auth type
*/
func (b BasicAuth) GetSecret() string {
	return b.Password
}

type ClientCredentials struct {
	ClientId     string // Client ID for the API
	ClientSecret string // Client Secret for the API
}

/*
Encode a client credentials type payload and return the payload string according to RFC 6749
*/
func (c ClientCredentials) Prepare() string {
	credQuery := url.Values{}
	credQuery.Add("grant_type", "client_credentials")
	credQuery.Add("client_id", c.ClientId)
	credQuery.Add("client_secret", c.ClientSecret)
	return credQuery.Encode()

}

// return the client ID
func (c ClientCredentials) GetPublic() string {
	return c.ClientId
}

// Return the Client Secret
func (c ClientCredentials) GetSecret() string {
	return c.ClientSecret
}

type SshKey struct {
	User       string
	PrivateKey string
}

func (s SshKey) GetPublic() string {
	return s.User
}
func (s SshKey) GetSecret() string {
	return s.PrivateKey
}
func (s SshKey) Prepare() string {
	return s.PrivateKey
}

type ApiKeyRing struct {
	Rungs []DaemonKeyRing
	Keys  map[string]Key // hashmap with the keys in the keyring. Protected with getters and setters
}

/*
Retrieve a keyring from the daemon keyring

	:param name: the name of the keyring. e.g., 'LINODE', or 'OPENWRT'
*/
func (a *ApiKeyRing) GetKey(name string) (Key, error) {
	var key Key
	key, ok := a.Keys[name]
	if ok {
		return key, nil
	}
	if len(a.Rungs) > 0 {
		for i := range a.Rungs {
			fmt.Println("trying to get key: " + name)
			key, err := a.Rungs[i].GetKey(name)
			if err == nil {
				a.AddKey(name, key)
				return key, nil
			}

		}
	}
	return key, &KeyNotFound{Name: name}
}

/*
Add a key to the daemon keyring

	:param name: name to give the key, used when indexing
	:param key: the Key struct to add to the keyring
*/
func (a *ApiKeyRing) AddKey(name string, key Key) error {
	_, ok := a.Keys[name]
	if ok {
		return &KeyExists{Name: name}
	}
	a.Keys[name] = key
	return nil
}

/*
Remove a key from the daemon keyring

	:param name: the name that the key was given when adding to the keyring
*/
func (a *ApiKeyRing) RemoveKey(name string) error {
	_, err := a.GetKey(name)
	if err != nil {
		return err
	}
	delete(a.Keys, name)
	return nil
}

// Return the resource name for logging purposes
func (a *ApiKeyRing) Source() string {
	return "Base API Keyring"
}

/*
Create a new daemon keyring. Passing additional implementers of the DaemonKeyRing will
allow the GetKey() method on the toplevel keyring to search all subsequent keyrings for a match.
*/
func NewKeyRing() *ApiKeyRing {
	return &ApiKeyRing{
		Keys:  map[string]Key{},
		Rungs: []DaemonKeyRing{},
	}

}

/*
Function to wrap GetKey that will return an ActionOut implementer
*/
func (a *ApiKeyRing) KeyringRouter(arg ActionIn) (ActionOut, error) {
	var out KeyGetterActionOut
	switch arg.Method() {
	case "show":
		switch arg.Arg() {
		case "all":
			// unimplemented
			return out, nil
		case "":
			return out, &InvalidAction{Msg: "No argument passed!"}
		default:
			key, err := a.GetKey(arg.Arg())

			if err != nil {
				return out, err
			}
			out = KeyGetterActionOut{Public: key.GetPublic(), Private: key.GetSecret()}

			return out, nil

		}
	}
	return out, &InvalidAction{Msg: "No method resolved!"}
}

/*
Bootstrap the keyring
*/
func (a *ApiKeyRing) Bootstrap() error { return nil }

/*

######################
##### INTERFACES #####
######################

*/

type DaemonKeyRing interface {
	GetKey(string) (Key, error) // retrieve a key by its tag on the keyring
	AddKey(string, Key) error   // Add a key to your keyring
	RemoveKey(string) error     // Remove a key from the keyring
	Source() string             // Return the name of the resource being called, i.e. 'Semaphone Keystore', or 'Hashicorp Vault'
}

type Key interface {
	// This function is supposed to return the payload for the given key type according to its RFC
	// i.e. if the 'type' is Bearer, then it returns a string with 'Bearer tokencode123456xyz'
	Prepare() string
	GetPublic() string // Get the public identifier of the key, i.e. the username, or client id, etc.
	GetSecret() string // Get the private/secret data, i.e. the password, API key, client secret, etc
}

/*

######################
##### ERRORS #####
######################

*/

type KeyNotFound struct {
	Name string
}

func (k *KeyNotFound) Error() string {
	return fmt.Sprintf("Key with name '%s' was not found in the daemon keyring", k.Name)
}

type KeyExists struct {
	Name string
}

func (k *KeyExists) Error() string {
	return fmt.Sprintf("Key with name '%s' already exists in the keyring", k.Name)
}
