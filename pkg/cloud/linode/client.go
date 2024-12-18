package linode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	daemonproto "git.aetherial.dev/aeth/yosai/pkg/daemon-proto"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
	"git.aetherial.dev/aeth/yosai/pkg/secrets/keyring"
)

const LinodeApiUrl = "api.linode.com"
const LinodeInstances = "linode/instances"
const LinodeImages = "images"
const LinodeApiVers = "v4"
const LinodeRegions = "regions"
const LinodeTypes = "linode/types"

const (
	DEFAULT_TYPE   = "g6-nanode-1"
	DEFAULT_IMAGE  = "linode/debian11"
	DEFAULT_REGION = "us-southeast"
)

type GetAllLinodes struct {
	Data []GetLinodeResponse `json:"data"`
}

type GetLinodeResponse struct {
	Id      int      `json:"id"`
	Ipv4    []string `json:"ipv4"`
	Label   string   `json:"label"`
	Created string   `json:"created"`
	Region  string   `json:"region"`
	Status  string   `json:"status"`
}

type TypesResponse struct {
	Data []TypesResponseInner `json:"data"`
}

type TypesResponseInner struct {
	Id string `json:"id"`
}

type ImagesResponse struct {
	Data []ImagesResponseInner `json:"data"`
}

type ImagesResponseInner struct {
	Id string `json:"id"`
}

type RegionsResponse struct {
	Data []RegionResponseInner `json:"data"`
}

type RegionResponseInner struct {
	Id string `json:"id"`
}

type NewLinodeBody struct {
	Label          string   `json:"label"`
	AuthorizedKeys []string `json:"authorized_keys"`
	Booted         bool     `json:"booted"`
	Image          string   `json:"image"`
	RootPass       string   `json:"root_pass"`
	Region         string   `json:"region"`
	Type           string   `json:"type"`
}

type LinodeConnection struct {
	Client    *http.Client
	Keyring   keyring.DaemonKeyRing
	KeyTagger keytags.Keytagger
	Config    *config.Configuration
}

// Logging wrapper
func (ln LinodeConnection) Log(msg ...string) {
	lnMsg := []string{"LinodeConnection:"}
	lnMsg = append(lnMsg, msg...)
	ln.Config.Log(lnMsg...)
}

// Construct a NewLinodeBody struct for a CreateNewLinode call
func NewLinodeBodyBuilder(image string, region string, linodeType string, label string, keyring keyring.DaemonKeyRing) (NewLinodeBody, error) {
	var newLnBody NewLinodeBody
	rootPass, err := keyring.GetKey(keytags.VPS_ROOT_PASS_KEYNAME)
	if err != nil {
		return newLnBody, &LinodeClientError{Msg: err.Error()}
	}
	rootSshKey, err := keyring.GetKey(keytags.VPS_SSH_KEY_KEYNAME)
	if err != nil {
		return newLnBody, &LinodeClientError{Msg: err.Error()}
	}

	return NewLinodeBody{AuthorizedKeys: []string{rootSshKey.GetPublic()},
		Label:    label,
		RootPass: rootPass.GetSecret(),
		Booted:   true,
		Image:    image,
		Region:   region,
		Type:     linodeType}, nil
}

/*
Get all regions that a server can be deployed in from Linode

	:param keyring: a keyring.DaemonKeyRing implementer that is able to return a linode API key
*/
func (ln LinodeConnection) GetRegions() (RegionsResponse, error) {
	var regions RegionsResponse
	b, err := ln.Get(LinodeRegions)
	if err != nil {
		return regions, err
	}
	err = json.Unmarshal(b, &regions)
	if err != nil {
		return regions, err
	}
	return regions, nil

}

/*
Get all of the available image types from linode

	:param keyring: a keyring.DaemonKeyRing interface implementer. Responsible for getting the linode API key
*/
func (ln LinodeConnection) GetImages() (ImagesResponse, error) {
	var imgResp ImagesResponse
	b, err := ln.Get(LinodeImages)
	if err != nil {
		return imgResp, err
	}
	err = json.Unmarshal(b, &imgResp)
	if err != nil {
		return imgResp, &LinodeClientError{Msg: err.Error()}

	}
	return imgResp, nil

}

/*
Get all of the available Linode types from linode

	:param keyring: a keyring.DaemonKeyRing interface implementer. Responsible for getting the linode API key
*/
func (ln LinodeConnection) GetTypes() (TypesResponse, error) {
	var typesResp TypesResponse
	b, err := ln.Get(LinodeTypes)
	if err != nil {
		return typesResp, err
	}
	err = json.Unmarshal(b, &typesResp)
	if err != nil {
		return typesResp, &LinodeClientError{Msg: err.Error()}

	}
	return typesResp, nil

}

/*
Get a Linode by its ID, used for assertion when deleting an old linode
*/
func (ln LinodeConnection) GetLinode(id string) (GetLinodeResponse, error) {
	var getLnResp GetLinodeResponse
	b, err := ln.Get(fmt.Sprintf("%s/%s", LinodeInstances, id))
	if err != nil {
		return getLnResp, err
	}
	err = json.Unmarshal(b, &getLnResp)
	if err != nil {
		return getLnResp, &LinodeClientError{Msg: err.Error()}
	}
	return getLnResp, nil
}

/*
List all linodes on your account

	:param keyring: a keyring.DaemonKeyRing implementer that can return the linode API key
*/
func (ln LinodeConnection) ListLinodes() (GetAllLinodes, error) {
	var allLinodes GetAllLinodes
	b, err := ln.Get(LinodeInstances)
	if err != nil {
		return allLinodes, err
	}
	err = json.Unmarshal(b, &allLinodes)
	if err != nil {
		return allLinodes, &LinodeClientError{Msg: err.Error()}
	}
	return allLinodes, nil
}

/*
Get linode by IP Address

	:param addr: the IPv4 address of your linode
*/
func (ln LinodeConnection) GetByIp(addr string) (GetLinodeResponse, error) {
	var out GetLinodeResponse
	servers, err := ln.ListLinodes()
	if err != nil {
		return out, err
	}
	for i := range servers.Data {
		if servers.Data[i].Ipv4[0] == addr {
			return servers.Data[i], nil
		}
	}
	return out, &LinodeClientError{Msg: "Linode with Address of: " + addr + " not found."}

}

/*
Get a linode by its name/label

	:param name: the name/label of the linode
*/
func (ln LinodeConnection) GetByName(name string) (GetLinodeResponse, error) {
	var out GetLinodeResponse
	servers, err := ln.ListLinodes()
	if err != nil {
		return out, err
	}
	for i := range servers.Data {
		if servers.Data[i].Label == name {
			return servers.Data[i], nil
		}
	}
	return out, &LinodeClientError{Msg: "Linode with name: " + name + " not found."}

}

/*
Create a new linode instance

	    :param keyring: a keyring.DaemonKeyRing implementer that can return a linode API key
		:param body: the request body for the new linode request
*/
func (ln LinodeConnection) CreateNewLinode(body NewLinodeBody) (GetLinodeResponse, error) {
	var newLnResp GetLinodeResponse
	reqBody, err := json.Marshal(&body)
	if err != nil {
		return newLnResp, err
	}
	apiKey, err := ln.Keyring.GetKey(ln.KeyTagger.LinodeApiKeyname())
	if err != nil {
		return newLnResp, &LinodeClientError{Msg: err.Error()}
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/%s/%s", LinodeApiUrl, LinodeApiVers, LinodeInstances), bytes.NewReader(reqBody))
	req.Header.Add("Authorization", apiKey.Prepare())
	req.Header.Add("Content-Type", "application/json")
	resp, err := ln.Client.Do(req)
	if err != nil {
		return newLnResp, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return newLnResp, &LinodeClientError{Msg: err.Error()}
	}
	if resp.StatusCode != 200 {
		return newLnResp, &LinodeClientError{Msg: resp.Status + "\n" + string(b)}
	}
	err = json.Unmarshal(b, &newLnResp)
	if err != nil {
		return newLnResp, &LinodeClientError{Msg: err.Error()}
	}
	return newLnResp, nil

}

/*
Delete a linode instance. Internally, this function will check that the linode ID exists before deleting

	:param id: the id of the linode.
*/
func (ln LinodeConnection) DeleteLinode(id string) error {
	_, err := ln.GetLinode(id)
	if err != nil {
		return &LinodeClientError{Msg: err.Error()}
	}
	_, err = ln.Delete(fmt.Sprintf("%s/%s", LinodeInstances, id))
	if err != nil {
		return &LinodeClientError{Msg: err.Error()}
	}
	return nil
}

/*
Agnostic GET method for calling the upstream linode server

	:param keyring: a keyring.DaemonKeyRing implementer to get the linode API key from
	:param path: the path to GET, added into the base API url
*/
func (ln LinodeConnection) Get(path string) ([]byte, error) {
	var b []byte
	apiKey, err := ln.Keyring.GetKey(ln.KeyTagger.LinodeApiKeyname())
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/%s/%s", LinodeApiUrl, LinodeApiVers, strings.TrimPrefix(path, "/")), nil)
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	req.Header.Add("Authorization", apiKey.Prepare())
	resp, err := ln.Client.Do(req)
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	return b, nil

}

/*
Agnostic DELETE method for deleting a resource from Linode

	:param keyring: a keyring.DaemonKeyRing implementer for getting the linode API key
	:param path: the path to perform the DELETE method on
*/
func (ln LinodeConnection) Delete(path string) ([]byte, error) {
	var b []byte
	apiKey, err := ln.Keyring.GetKey(ln.KeyTagger.LinodeApiKeyname())
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	req, err := http.NewRequest("DELETE", fmt.Sprintf("https://%s/%s/%s", LinodeApiUrl, LinodeApiVers, strings.TrimPrefix(path, "/")), nil)
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	req.Header.Add("Authorization", apiKey.Prepare())
	resp, err := ln.Client.Do(req)
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return b, &LinodeClientError{Msg: err.Error()}
	}
	return b, nil

}

/*
Poll for new server creation

	    :param name: the IPv4 address of the linode server
		:param max_tries: the number of calls the client will send to linode before exiting
*/
func (ln LinodeConnection) ServerPoll(name string, max_tries int) error {
	var count int
	for {
		count = count + 1
		if count > max_tries {
			return &LinodeTimeOutError{Tries: max_tries}
		}
		ln.Log("Polling for server status times: ", fmt.Sprint(count))
		resp, err := ln.GetByName(name)
		if err != nil {
			return err
		}
		if resp.Status == "running" {
			ln.Log("Server: ", resp.Ipv4[0], " showing as: ", resp.Status)
			return nil
		}
		ln.Log("Server inactive, showing status: ", resp.Status)

		time.Sleep(time.Second * 3)
	}

}

/*
Bootstrap the cloud environment
*/
func (ln LinodeConnection) Bootstrap() error { return nil }

/*
############################################
########### DAEMON EVENT HANDLERS ##########
############################################
*/

type DeleteLinodeRequest struct {
	Name string `json:"name"`
	Id   string `json:"id"`
}

type AddLinodeRequest struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Region string `json:"region"`
	Type   string `json:"type"`
}

type PollLinodeRequest struct {
	Address string `json:"address"`
}

func (ln LinodeConnection) DeleteLinodeHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var req DeleteLinodeRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}

	resp, err := ln.GetByName(req.Name)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}

	err = ln.DeleteLinode(fmt.Sprint(resp.Id))
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_ACCEPTED, []byte(err.Error()))
	}
	responseMessage := []byte("Server: " + fmt.Sprint(resp.Id) + " was deleted.")
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, responseMessage)

}

/*
Wraps the creation of a linode to make the LinodeRouter function slimmer

	:param msg: a daemonproto.SockMessage struct with request info
*/
func (ln LinodeConnection) AddLinodeHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	ln.Log("Recieved request to create a new linode server.")
	var payload AddLinodeRequest
	err := json.Unmarshal(msg.Body, &payload)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	newLinodeReq, err := NewLinodeBodyBuilder(payload.Image,
		payload.Region,
		payload.Type,
		payload.Name,
		ln.Keyring)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	resp, err := ln.CreateNewLinode(newLinodeReq)
	if err != nil {
		ln.Log("There was an error creating server: ", payload.Name, err.Error())
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	ln.Log("Server: ", payload.Name, " Created successfully.")

	b, _ := json.Marshal(resp)
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, b)

}

/*
Wraps the polling feature of the client in a Handler function

	:param msg: a daemonproto.SockMessage that contains the request info
*/
func (ln LinodeConnection) PollLinodeHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	var req PollLinodeRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_TIMEOUT, []byte(err.Error()))
	}

	err = ln.ServerPoll(req.Address, 60)
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_TIMEOUT, []byte(err.Error()))
	}
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, []byte("Server is running."))

}

/*
Wraps the show servers functionality in a the correct Route interface

	:param msg: a daemonproto.SockMessage that contains a request
*/
func (ln LinodeConnection) ShowLinodeHandler(msg daemonproto.SockMessage) daemonproto.SockMessage {
	servers, err := ln.ListLinodes()
	if err != nil {
		return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_FAILED, []byte(err.Error()))
	}
	b, _ := json.Marshal(servers)
	return *daemonproto.NewSockMessage(daemonproto.MsgResponse, daemonproto.REQUEST_OK, b)

}

type LinodeRouter struct {
	routes map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage
}

func (l *LinodeRouter) Register(method daemonproto.Method, callable func(daemonproto.SockMessage) daemonproto.SockMessage) {
	l.routes[method] = callable
}

func (l *LinodeRouter) Routes() map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage {
	return l.routes
}

func NewLinodeRouter() *LinodeRouter {
	return &LinodeRouter{routes: map[daemonproto.Method]func(daemonproto.SockMessage) daemonproto.SockMessage{}}
}

/*
#####################
####### ERRORS ######
#####################
*/
type LinodeClientError struct {
	Msg string
}

func (ln *LinodeClientError) Error() string {
	return fmt.Sprintf("There was an error calling linode: '%s'", ln.Msg)
}

type LinodeTimeOutError struct {
	Tries int
}

func (ln *LinodeTimeOutError) Error() string {
	return "Polling timed out after: " + fmt.Sprint(ln.Tries) + " attempts"
}
