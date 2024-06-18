package linode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"git.aetherial.dev/aeth/yosai/pkg/keytags"
)

const LinodeApiUrl = "api.linode.com"
const LinodeInstances = "linode/instances"
const LinodeApiVers = "v4"
const LinodeRegions = "regions"

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
	Region         string   `json:"region"`
	Type           string   `json:"type"`
}

type LinodeConnection struct {
	Client *http.Client
}

// Construct a NewLinodeBody struct for a CreateNewLinode call
func NewLinodeBodyBuilder(image string, region string, linodeType string) (NewLinodeBody, error) {
	return NewLinodeBody{}, nil
}

func (ln LinodeConnection) GetRegions(keyring daemon.DaemonKeyRing) (RegionsResponse, error) {
	var regions RegionsResponse
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/%s/%s", LinodeApiUrl, LinodeApiVers, LinodeRegions), nil)
	if err != nil {
		return regions, err
	}
	key, err := keyring.GetKey(keytags.LINODE_API_KEYNAME)
	if err != nil {
		return regions, err
	}
	req.Header.Add("Authorization", key.Prepare())
	resp, err := ln.Client.Do(req)
	if err != nil {
		return regions, &LinodeClientError{Msg: err.Error()}
	}
	if resp.StatusCode != 200 {
		return regions, &LinodeClientError{Msg: resp.Status}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return regions, err
	}
	err = json.Unmarshal(b, &regions)
	if err != nil {
		return regions, err
	}
	return regions, nil

}

func (ln LinodeConnection) CreateNewLinode(keyring daemon.DaemonKeyRing, body NewLinodeBody) error {
	b, err := json.Marshal(&body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/%s/%s", LinodeApiUrl, LinodeApiVers, LinodeInstances), bytes.NewReader(b))
	resp, err := ln.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &LinodeClientError{Msg: resp.Status}
	}
	return nil

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
