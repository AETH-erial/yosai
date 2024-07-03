package linode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
)

const LinodeApiUrl = "api.linode.com"
const LinodeInstances = "linode/instances"
const LinodeImages = "images"
const LinodeApiVers = "v4"
const LinodeRegions = "regions"
const LinodeTypes = "linode/types"

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
	AuthorizedKeys []string `json:"authorized_keys"`
	Booted         bool     `json:"booted"`
	Image          string   `json:"image"`
	RootPass       string   `json:"root_pass"`
	Region         string   `json:"region"`
	Type           string   `json:"type"`
}

type LinodeConnection struct {
	Client *http.Client
}

// Construct a NewLinodeBody struct for a CreateNewLinode call
func NewLinodeBodyBuilder(image string, region string, linodeType string, keyring daemon.DaemonKeyRing) (NewLinodeBody, error) {
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
		RootPass: rootPass.GetSecret(),
		Booted:   true,
		Image:    image,
		Region:   region,
		Type:     linodeType}, nil
}

/*
Get all regions that a server can be deployed in from Linode

	:param keyring: a daemon.DaemonKeyRing implementer that is able to return a linode API key
*/
func (ln LinodeConnection) GetRegions(keyring daemon.DaemonKeyRing) (RegionsResponse, error) {
	var regions RegionsResponse
	b, err := ln.Get(keyring, LinodeRegions)
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

	:param keyring: a daemon.DaemonKeyRing interface implementer. Responsible for getting the linode API key
*/
func (ln LinodeConnection) GetImages(keyring daemon.DaemonKeyRing) (ImagesResponse, error) {
	var imgResp ImagesResponse
	b, err := ln.Get(keyring, LinodeImages)
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

	:param keyring: a daemon.DaemonKeyRing interface implementer. Responsible for getting the linode API key
*/
func (ln LinodeConnection) GetTypes(keyring daemon.DaemonKeyRing) (TypesResponse, error) {
	var typesResp TypesResponse
	b, err := ln.Get(keyring, LinodeTypes)
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
func (ln LinodeConnection) GetLinode(keyring daemon.DaemonKeyRing, id string) (GetLinodeResponse, error) {
	var getLnResp GetLinodeResponse
	b, err := ln.Get(keyring, fmt.Sprintf("%s/%s", LinodeInstances, id))
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

	:param keyring: a daemon.DaemonKeyRing implementer that can return the linode API key
*/
func (ln LinodeConnection) ListLinodes(keyring daemon.DaemonKeyRing) (GetAllLinodes, error) {
	var allLinodes GetAllLinodes
	b, err := ln.Get(keyring, LinodeInstances)
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
Create a new linode instance

	    :param keyring: a daemon.DaemonKeyRing implementer that can return a linode API key
		:param body: the request body for the new linode request
*/
func (ln LinodeConnection) CreateNewLinode(keyring daemon.DaemonKeyRing, body NewLinodeBody) (GetLinodeResponse, error) {
	var newLnResp GetLinodeResponse
	reqBody, err := json.Marshal(&body)
	if err != nil {
		return newLnResp, err
	}
	apiKey, err := keyring.GetKey(keytags.LINODE_API_KEYNAME)
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
		return newLnResp, &LinodeClientError{Msg: resp.Status}
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
func (ln LinodeConnection) DeleteLinode(keyring daemon.DaemonKeyRing, id string) error {
	_, err := ln.GetLinode(keyring, id)
	if err != nil {
		return &LinodeClientError{Msg: err.Error()}
	}
	_, err = ln.Delete(keyring, fmt.Sprintf("%s/%s", LinodeInstances, id))
	if err != nil {
		return &LinodeClientError{Msg: err.Error()}
	}
	return nil
}

/*
Agnostic GET method for calling the upstream linode server

	:param keyring: a daemon.DaemonKeyRing implementer to get the linode API key from
	:param path: the path to GET, added into the base API url
*/
func (ln LinodeConnection) Get(keyring daemon.DaemonKeyRing, path string) ([]byte, error) {
	var b []byte
	apiKey, err := keyring.GetKey(keytags.LINODE_API_KEYNAME)
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

	:param keyring: a daemon.DaemonKeyRing implementer for getting the linode API key
	:param path: the path to perform the DELETE method on
*/
func (ln LinodeConnection) Delete(keyring daemon.DaemonKeyRing, path string) ([]byte, error) {
	var b []byte
	apiKey, err := keyring.GetKey(keytags.LINODE_API_KEYNAME)
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
