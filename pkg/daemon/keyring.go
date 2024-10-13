package daemon

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"

	"git.aetherial.dev/aeth/yosai/pkg/keytags"
)

const (
	SSH_KEY            = "ssh"
	API_KEY            = "api_key"
	BEARER_AUTH        = "bearer_auth"
	CLIENT_CREDENTIALS = "client_credentials"
	BASIC_AUTH         = "basic_auth"
	LOGIN_CRED         = "login_password"
	WIREGUARD          = "wireguard"
)

type WireguardKeypair struct {
	PrivateKey string
	PublicKey  string
}

func (w WireguardKeypair) GetPublic() string {
	return w.PublicKey
}
func (w WireguardKeypair) GetSecret() string {
	return w.PrivateKey
}
func (w WireguardKeypair) Prepare() string {
	return ""
}
func (w WireguardKeypair) GetType() string {
	return WIREGUARD
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

func (b BearerAuth) GetType() string {
	return BEARER_AUTH
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

func (b BasicAuth) GetType() string {
	return BASIC_AUTH
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
func (c ClientCredentials) GetType() string {
	return CLIENT_CREDENTIALS
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
func (s SshKey) GetType() string {
	return SSH_KEY
}

type ApiKeyRing struct {
	Rungs     []DaemonKeyRing
	Keys      map[string]Key // hashmap with the keys in the keyring. Protected with getters and setters
	Config    *Configuration
	KeyTagger keytags.Keytagger
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
			key, err := a.Rungs[i].GetKey(name)
			if err != nil {
				if errors.Is(err, KeyNotFound) {
					a.Log("Key: ", name, "not found.", err.Error())
					continue
				}
				if errors.Is(err, KeyRingError) {
					a.Log("Error getting key:", name, err.Error())
					return key, err
				}
				a.Log("Unhandled exception getting key: ", name, err.Error())
				log.Fatal("Ungraceful shutdown. unhandled error within keyring: ", err)

			}

			if key.GetPublic() == "" || key.GetSecret() == "" {
				a.Log("null key: ", name, "returned.")
				continue
			}
			a.Log("Key:", name, "successfully added to the daemon keyring.")
			a.AddKey(name, key)
			return key, nil

		}
	}
	return key, KeyNotFound
}

/*
Add a key to the daemon keyring

	:param name: name to give the key, used when indexing
	:param key: the Key struct to add to the keyring
*/
func (a *ApiKeyRing) AddKey(name string, key Key) error {
	_, ok := a.Keys[name]
	if ok {
		return KeyExists
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
func NewKeyRing(cfg *Configuration, keytagger keytags.Keytagger) *ApiKeyRing {
	return &ApiKeyRing{
		Keys:      map[string]Key{},
		Rungs:     []DaemonKeyRing{},
		Config:    cfg,
		KeyTagger: keytagger,
	}

}

func (a *ApiKeyRing) Log(msg ...string) {
	keyMsg := []string{
		"ApiKeyRing:",
	}
	keyMsg = append(keyMsg, msg...)
	a.Config.Log(keyMsg...)
}

type KeyringRequest struct {
	Public string `json:"public"`
	Secret string `json:"secret"`
	Type   string `json:"type"`
	Name   string `json:"name"`
}

/*
Wrapping the show keyring function in a route friendly interface

	:param msg: a message to be decoded from the daemon socket
*/
func (a *ApiKeyRing) ShowKeyringHandler(msg SockMessage) SockMessage {
	var req KeyringRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	switch req.Name {
	case "all":
		b, err := json.Marshal(a.Keys)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		return *NewSockMessage(MsgResponse, REQUEST_OK, b)
	default:
		key, err := a.GetKey(req.Name)
		if err != nil {
			return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
		}
		b, _ := json.Marshal(key)
		return *NewSockMessage(MsgResponse, REQUEST_OK, b)
	}
}

/*
Wrapping the reload keyring function in a route friendly interface

	:param msg: a message to be decoded from the daemon socket
*/
func (a *ApiKeyRing) ReloadKeyringHandler(msg SockMessage) SockMessage {
	protectedKeys := a.KeyTagger.ProtectedKeys()
	keynames := []string{}
	for keyname := range a.Keys {
		_, ok := protectedKeys[keyname]
		if ok {
			continue
		}
		keynames = append(keynames, keyname)
	}
	for i := range keynames {
		delete(a.Keys, keynames[i])
	}
	a.Log("Keyring depleted, keys in keyring: ", fmt.Sprint(len(a.Keys)))
	a.Log("Keys to retrieve: ", fmt.Sprint(keynames))
	for i := range keynames {
		_, err := a.GetKey(keynames[i])
		if err != nil {
			a.Log("Keyring reload error, Error getting key: ", keynames[i], err.Error())
		}
	}
	return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Keyring successfully reloaded."))
}

/*
Wrapping the bootstrap keyring function in a route friendly interface

	:param msg: a message to be decoded from the daemon socket
*/
func (a *ApiKeyRing) BootstrapKeyringHandler(msg SockMessage) SockMessage {
	err := a.Bootstrap()
	if err != nil {
		return *NewSockMessage(MsgResponse, REQUEST_FAILED, []byte(err.Error()))
	}
	return *NewSockMessage(MsgResponse, REQUEST_OK, []byte("Keyring successfully bootstrapped."))
}

/*
Bootstrap the keyring
*/
func (a *ApiKeyRing) Bootstrap() error {
	allkeytags := a.KeyTagger.AllKeys()
	for i := range allkeytags {
		kn := allkeytags[i]
		_, err := a.GetKey(kn)
		if err != nil {
			return &KeyringBootstrapError{Msg: "Key with keytag: " + kn + " was not found on any of the daemon Keyring rungs."}
		}
	}
	return nil

}

type KeyringRouter struct {
	routes map[Method]func(SockMessage) SockMessage
}

func (k *KeyringRouter) Register(method Method, callable func(SockMessage) SockMessage) {
	k.routes[method] = callable
}

func (k *KeyringRouter) Routes() map[Method]func(SockMessage) SockMessage {
	return k.routes
}

func NewKeyRingRouter() *KeyringRouter {
	return &KeyringRouter{routes: map[Method]func(SockMessage) SockMessage{}}
}

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
	GetType() string   // Returns the type of key. I.e. API_KEY, SSH_KEY, BASIC_AUTH, etc
}

/*

######################
##### ERRORS #####
######################

*/

var (
	KeyNotFound  = errors.New("Key not found.")
	KeyExists    = errors.New("Key exists.")
	KeyRingError = errors.New("Unexpected error from child keyrung")
)

type KeyringBootstrapError struct {
	Msg string
}

func (k *KeyringBootstrapError) Error() string {
	return fmt.Sprintf("There was a fatal error bootstrapping the keyring: %s", k.Msg)
}
