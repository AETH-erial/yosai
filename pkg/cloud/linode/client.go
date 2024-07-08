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

func (g GetAllLinodes) GetResult() string {
	resp, err := json.MarshalIndent(&g, " ", "    ")
	if err != nil {
		return "Sorry, couldnt parse the json." + err.Error()
	}
	return string(resp)
}

type GetLinodeResponse struct {
	Id      int      `json:"id"`
	Ipv4    []string `json:"ipv4"`
	Label   string   `json:"label"`
	Created string   `json:"created"`
	Region  string   `json:"region"`
	Status  string   `json:"status"`
}

/*
implementing the daemon.ActionOut interface
*/
func (g GetLinodeResponse) GetResult() string {
	b, err := json.MarshalIndent(g, " ", "    ")
	if err != nil {
		return "Error unmarshalling response: " + err.Error()
	}
	return string(b)
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
	Client  *http.Client
	Keyring daemon.DaemonKeyRing
	Config  daemon.Configuration
}

// Construct a NewLinodeBody struct for a CreateNewLinode call
func NewLinodeBodyBuilder(image string, region string, linodeType string, label string, keyring daemon.DaemonKeyRing) (NewLinodeBody, error) {
	var newLnBody NewLinodeBody
	rootPass, err := keyring.GetKey(keytags.VPS_ROOT_PASS_KEYNAME)
	if err != nil {
		return newLnBody, &LinodeClientError{Msg: err.Error()}
	}
	rootSshKey, err := keyring.GetKey(keytags.VPS_SSH_KEY_KEYNAME)
	if err != nil {
		return newLnBody, &LinodeClientError{Msg: err.Error()}
	}
	fmt.Print(rootSshKey.GetPublic(), rootSshKey.GetSecret(), "\n")

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

	:param keyring: a daemon.DaemonKeyRing implementer that is able to return a linode API key
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

	:param keyring: a daemon.DaemonKeyRing interface implementer. Responsible for getting the linode API key
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

	:param keyring: a daemon.DaemonKeyRing interface implementer. Responsible for getting the linode API key
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

	:param keyring: a daemon.DaemonKeyRing implementer that can return the linode API key
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
Create a new linode instance

	    :param keyring: a daemon.DaemonKeyRing implementer that can return a linode API key
		:param body: the request body for the new linode request
*/
func (ln LinodeConnection) CreateNewLinode(body NewLinodeBody) (GetLinodeResponse, error) {
	var newLnResp GetLinodeResponse
	reqBody, err := json.Marshal(&body)
	if err != nil {
		return newLnResp, err
	}
	apiKey, err := ln.Keyring.GetKey(keytags.LINODE_API_KEYNAME)
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

	:param keyring: a daemon.DaemonKeyRing implementer to get the linode API key from
	:param path: the path to GET, added into the base API url
*/
func (ln LinodeConnection) Get(path string) ([]byte, error) {
	var b []byte
	apiKey, err := ln.Keyring.GetKey(keytags.LINODE_API_KEYNAME)
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
func (ln LinodeConnection) Delete(path string) ([]byte, error) {
	var b []byte
	apiKey, err := ln.Keyring.GetKey(keytags.LINODE_API_KEYNAME)
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
Bootstrap the cloud environment
*/
func (ln LinodeConnection) Bootstrap() error { return nil }

/*
############################################
#### IMPLEMENTING DAEMON CLI INTERFACES ####
############################################
*/
type LinodeActionOut struct {
	Content string
}

func (lno LinodeActionOut) GetResult() string {
	return lno.Content
}

func (ln LinodeConnection) LinodeRouter(action daemon.ActionIn) (daemon.ActionOut, error) {
	var out LinodeActionOut

	switch action.Method() {
	case "show":
		switch action.Arg() {
		case "all":
			servers, err := ln.ListLinodes()
			if err != nil {
				return out, err
			}
			return servers, nil
		case "images":
			imgs, err := ln.GetImages()
			if err != nil {
				return out, err
			}
			b, err := json.MarshalIndent(imgs, " ", "    ")
			if err != nil {
				return out, err
			}
			return LinodeActionOut{Content: string(b)}, nil
		case "regions":

			regions, err := ln.GetRegions()
			if err != nil {
				return out, err
			}
			b, err := json.MarshalIndent(regions, " ", "    ")
			if err != nil {
				return out, err
			}
			return LinodeActionOut{Content: string(b)}, nil
		case "types":

			types, err := ln.GetTypes()
			if err != nil {
				return out, err
			}
			b, err := json.MarshalIndent(types, " ", "    ")
			if err != nil {
				return out, err
			}
			return LinodeActionOut{Content: string(b)}, nil

		}
	case "rm":
		switch action.Arg() {
		case "":
			return out, &daemon.InvalidAction{Msg: "Not enough args passed."}
		default:
			err := ln.DeleteLinode(action.Arg())
			if err != nil {
				return out, err
			}
			return LinodeActionOut{Content: "Linode: " + action.Arg() + " deleted successfully."}, nil

		}
	case "new":
		fmt.Print(ln.Config.Image(), ln.Config.Region(), ln.Config.LinodeType(), "\n")
		body, err := NewLinodeBodyBuilder(ln.Config.Image(), ln.Config.Region(), ln.Config.LinodeType(), action.Arg(), ln.Keyring)
		if err != nil {
			return out, &daemon.InvalidAction{Msg: "Could not create the payload for making a new VM: " + err.Error()}
		}
		resp, err := ln.CreateNewLinode(body)
		if err != nil {
			return out, &daemon.InvalidAction{Msg: "Error occured when cteating a new VM: " + err.Error()}
		}
		return LinodeActionOut{Content: "New server is provisioning. ID: " + fmt.Sprint(resp.Id)}, nil

	}
	return LinodeActionOut{Content: "Unresolved method."}, nil
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
