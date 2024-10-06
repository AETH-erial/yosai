package memory

import (
	"encoding/json"
	"errors"
	"fmt"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
)

type MemoryKeyring struct {
	Keys      map[string]daemon.Key
	Rungs     []daemon.DaemonKeyRing
	KeyTagger keytags.Keytagger
	Config    daemon.Configuration
}

func (m *MemoryKeyring) GetKey(name string) (daemon.Key, error) {
	var key daemon.Key
	key, ok := m.Keys[name]
	if ok {
		return key, nil
	}
	return key, KeyNotFound

}
func (m *MemoryKeyring) AddKey(name string, key daemon.Key) error {
	_, ok := m.Keys[name]
	if ok {
		return KeyExists
	}
	m.Keys[name] = key
	return nil

}
func (m *MemoryKeyring) RemoveKey(name string) error {

	_, err := m.GetKey(name)
	if err != nil {
		return err
	}
	delete(m.Keys, name)
	return nil
}
func (m *MemoryKeyring) Source() string {
	return "In Memory Keyring"
}
func (m *MemoryKeyring) AddRung(rung daemon.DaemonKeyRing) {
	m.Rungs = append(m.Rungs, rung)
}
func (m *MemoryKeyring) Bootstrap() error {
	return nil

}
func (m *MemoryKeyring) Log(msg ...string) {
	keyMsg := []string{
		"MemoryKeyring:",
	}
	keyMsg = append(keyMsg, msg...)
	m.Config.Log(keyMsg...)
}
func (m *MemoryKeyring) Router(msg daemon.SockMessage) daemon.SockMessage {
	switch msg.Method {
	case "show":
		var req daemon.KeyringRequest
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_FAILED, []byte(err.Error()))
		}
		switch req.Name {
		case "all":
			b, err := json.Marshal(m.Keys)
			if err != nil {
				return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_FAILED, []byte(err.Error()))
			}
			return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_OK, b)
		default:
			key, err := m.GetKey(req.Name)
			if err != nil {
				return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_FAILED, []byte(err.Error()))
			}
			b, _ := json.Marshal(key)
			return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_OK, b)
		}
	case "reload":
		protectedKeys := m.KeyTagger.ProtectedKeys()
		keynames := []string{}
		for keyname := range m.Keys {
			_, ok := protectedKeys[keyname]
			if ok {
				continue
			}
			keynames = append(keynames, keyname)
		}
		for i := range keynames {
			delete(m.Keys, keynames[i])
		}
		m.Log("Keyring depleted, keys in keyring: ", fmt.Sprint(len(m.Keys)))
		m.Log("Keys to retrieve: ", fmt.Sprint(keynames))
		for i := range keynames {
			_, err := m.GetKey(keynames[i])
			if err != nil {
				m.Log("Keyring reload error, Error getting key: ", keynames[i], err.Error())
			}
		}
		return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_OK, []byte("Keyring successfully reloaded."))
	case "add":
		var req daemon.KeyringRequest
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_FAILED, []byte(err.Error()))
		}
		m.AddKey(req.Name, req)
		return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_OK, []byte("Key: "+req.Name+" added."))

	case "bootstrap":
		err := m.Bootstrap()
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_FAILED, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_OK, []byte("Keyring successfully bootstrapped."))
	default:
		return *daemon.NewSockMessage(daemon.MsgResponse, daemon.REQUEST_UNRESOLVED, []byte("Unresolvable method"))
	}

}

var (
	KeyNotFound  = errors.New("Key not found.")
	KeyExists    = errors.New("Key exists.")
	KeyRingError = errors.New("Unexpected error from child keyrung")
)
