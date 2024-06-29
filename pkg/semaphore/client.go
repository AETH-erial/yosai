package semaphore

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

const ProjectPath = "api/projects"

type SemaphoreConnection struct {
	Client    *http.Client
	ServerUrl string
	HttpProto string
}

type newProjectReqeust struct {
	Name             string `json:"name"`
	Alert            bool   `json:"alert"`
	AlertChat        string `json:"alert_chat"`
	MaxParallelTasks int    `json:"max_parallel_tasks"`
}

func NewSemaphoreClient(url string, proto string, log io.Writer) SemaphoreConnection {
	log.Write([]byte("Using HTTP mode: " + proto))
	return SemaphoreConnection{
		Client:    &http.Client{},
		ServerUrl: url,
		HttpProto: proto,
	}
}

/*
Create a new 'Project' in Semaphore

	:param name: the name to assign the project
*/
func (s SemaphoreConnection) NewProject(keyring daemon.DaemonKeyRing, name string) error {
	var b []byte
	var newProj newProjectReqeust
	newProj = newProjectReqeust{
		Name:             name,
		Alert:            false,
		AlertChat:        "",
		MaxParallelTasks: 0,
	}
	b, err := json.Marshal(&newProj)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	_, err = s.Post(keyring, ProjectPath, bytes.NewReader(b))
	if err != nil {
		return err
	}
	return nil

}

/*
Generic POST Request to sent to the Semaphore server

	    :param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
		:param path: the path to the API to POST. Preceeding slashes will be trimmed
		:param body: an io.Reader implementer to use as the POST body. Must comply with application/json Content-Type
*/
func (s SemaphoreConnection) Post(keyring daemon.DaemonKeyRing, path string, body io.Reader) ([]byte, error) {
	var b []byte
	apikey, err := keyring.GetKey(keytags.SEMAPHORE_API_KEYNAME)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s://%s/%s", s.HttpProto, s.ServerUrl, strings.TrimPrefix(path, "/")), body)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	req.Header.Add("Authorization", apikey.Prepare())
	req.Header.Add("Content-Type", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return b, &SemaphoreClientError{Msg: resp.Status}
	}
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	return b, nil

}

/*
Agnostic GET method for calling the upstream Semaphore server

	:param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
	:param path: the path to GET, added into the base API url
*/
func (s SemaphoreConnection) Get(keyring daemon.DaemonKeyRing, path string) ([]byte, error) {
	var b []byte
	apiKey, err := keyring.GetKey(keytags.SEMAPHORE_API_KEYNAME)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s://%s/%s", s.HttpProto, s.ServerUrl, strings.TrimPrefix(path, "/")), nil)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	req.Header.Add("Authorization", apiKey.Prepare())
	resp, err := s.Client.Do(req)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	defer resp.Body.Close()
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	return b, nil

}

/*
##########################################
################ ERRORS ##################
##########################################
*/

type SemaphoreClientError struct {
	Msg string
}

// Implementing error interface
func (s *SemaphoreClientError) Error() string {
	return fmt.Sprintf("There was an error with the call to the semaphore server: '%s'", s.Msg)
}
