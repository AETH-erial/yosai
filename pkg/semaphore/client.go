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

const ProjectsPath = "api/projects"
const ProjectPath = "api/project"
const YosaiProject = "Yosai VPN Sentinel"

type SemaphoreConnection struct {
	Client    *http.Client
	ServerUrl string
	HttpProto string
	ProjectId int
}

type ProjectsResponse struct {
	Id               int    `json:"id"`
	Name             string `json:"name"`
	Created          string `json:"created"`
	Alert            bool   `json:"alert"`
	AlertChat        string `json:"alert_chat"`
	MaxParallelTasks int    `json:"max_parallel_tasks"`
}

type newProjectReqeust struct {
	Name             string `json:"name"`
	Alert            bool   `json:"alert"`
	AlertChat        string `json:"alert_chat"`
	MaxParallelTasks int    `json:"max_parallel_tasks"`
}

type addRepoToProjectReq struct {
	Name      string `json:"name"`       // name of the project
	ProjectId int    `json:"project_id"` // the numerical ID of the project as per /api/project/<project id>
	GitUrl    string `json:"git_url"`    // the URL of the git repo (SSH address)
	GitBranch string `json:"git_branch"` // the branch to pull down
	SshKeyId  int    `json:"ssh_key_id"` // the numerical ID of the ssh key for the repository, as per /api/project/<project id>/keys
}

type addSshKeyReq struct {
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	ProjectId     int           `json:"project_id"`
	LoginPassword loginPassword `json:"login_password"`
	Ssh           sshKeyAdd     `json:"ssh"`
}

type KeyItemResponse struct {
	Id            int           `json:"id"`
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	ProjectId     int           `json:"project_id"`
	LoginPassword loginPassword `json:"login_password"`
	Ssh           sshKeyAdd     `json:"ssh"`
}

type loginPassword struct {
	Password string `json:"password"`
	Login    string `json:"login"`
}
type sshKeyAdd struct {
	PrivateKey string `json:"private_key"`
	Login      string `json:"login"`
}

/*
Create a new semaphore client

	    :param url: the base url of the semaphore server, without the HTTP/S prefix
		:param proto: either HTTP or HTTPS, depending on the server's SSL setup
		:param log: an io.Writer to write logfile to
		:param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
*/
func NewSemaphoreClient(url string, proto string, log io.Writer, keyring daemon.DaemonKeyRing) SemaphoreConnection {
	log.Write([]byte("Using HTTP mode: " + proto + "\n"))
	client := &http.Client{}
	semaphoreBootstrap := SemaphoreConnection{Client: client, ServerUrl: url, HttpProto: proto}

	id, err := semaphoreBootstrap.GetProjectByName(YosaiProject, keyring)
	if err != nil {
		log.Write([]byte(YosaiProject + " NOT FOUND IN SEMAPHORE. Creating..."))
		err = semaphoreBootstrap.NewProject(YosaiProject, keyring)
		if err != nil {
			log.Write([]byte("FATAL ERROR CREATING PROJECT. ABANDONING SHIP. Error: " + err.Error()))
		}
		id, _ = semaphoreBootstrap.GetProjectByName(YosaiProject, keyring)
		log.Write([]byte("Found " + YosaiProject + " with project id: " + fmt.Sprint(id)))
		return SemaphoreConnection{
			Client:    client,
			ServerUrl: url,
			HttpProto: proto,
			ProjectId: id,
		}
	}

	return SemaphoreConnection{
		Client:    &http.Client{},
		ServerUrl: url,
		HttpProto: proto,
		ProjectId: id,
	}
}

/*
Add SSH Key to a project

	:param name: the name to assign the key in the project
	:param keyring: a daemon.DaemonKeyRing implementer that can return the API key for Semaphore
	:param key: a daemon.Key implementer wrapping the SSH key
*/
func (s SemaphoreConnection) AddSshKey(name string, keyring daemon.DaemonKeyRing, key daemon.Key) error {
	keyAddReq := addSshKeyReq{
		Name:      name,
		Type:      "ssh",
		ProjectId: s.ProjectId,
		Ssh: sshKeyAdd{
			PrivateKey: key.GetSecret(),
			Login:      key.GetPublic(),
		},
	}
	b, err := json.Marshal(&keyAddReq)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	path := fmt.Sprintf("%s/%v/keys", ProjectPath, s.ProjectId)
	_, err = s.Post(keyring, path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	return nil

}

/*
Create a new 'Project' in Semaphore

	:param name: the name to assign the project
	:param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
*/
func (s SemaphoreConnection) NewProject(name string, keyring daemon.DaemonKeyRing) error {
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
	_, err = s.Post(keyring, ProjectsPath, bytes.NewReader(b))
	if err != nil {
		return err
	}
	return nil

}

/*
Add a repository to the project designated for the Yosai service

	    :param giturl: the url for the git repo containing the ansible scripts for VPN server config
		:param branch: the branch to target on the git repo
		:param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
*/
func (s SemaphoreConnection) AddRepository(giturl string, branch string, keyring daemon.DaemonKeyRing) error {
	key, err := s.GetSshKeyByName(keytags.GIT_SSH_KEYNAME, keyring)
	if err != nil {
		return err
	}
	repoAddRequest := addRepoToProjectReq{
		Name:      fmt.Sprintf("%s:%s", giturl, branch),
		ProjectId: s.ProjectId,
		GitUrl:    giturl,
		GitBranch: branch,
		SshKeyId:  key.Id,
	}
	b, err := json.Marshal(&repoAddRequest)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	_, err = s.Post(keyring, fmt.Sprintf("%s/%v/repositories", ProjectPath, s.ProjectId), bytes.NewReader(b))
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
Retrieve the projects in Semaphore

	:param keyring: a daemon.DaemonKeyRing implementer to get the API key from for Semaphore
*/
func (s SemaphoreConnection) GetProjects(keyring daemon.DaemonKeyRing) ([]ProjectsResponse, error) {
	var projectsResp []ProjectsResponse
	b, err := s.Get(keyring, ProjectsPath)
	if err != nil {
		return projectsResp, err
	}
	err = json.Unmarshal(b, &projectsResp)
	if err != nil {
		return projectsResp, &SemaphoreClientError{Msg: err.Error()}
	}
	return projectsResp, nil

}

/*
Get Project by its name, and return its ID
*/
func (s SemaphoreConnection) GetProjectByName(name string, keyring daemon.DaemonKeyRing) (int, error) {
	projects, err := s.GetProjects(keyring)
	if err != nil {
		return 0, err
	}
	for i := range projects {
		if projects[i].Name == name {
			return projects[i].Id, nil
		}
	}
	return 0, &SemaphoreClientError{Msg: fmt.Sprintf("Project with name: '%s' not found.", name)}
}

/*
Get SSH Keys from the current project
*/
func (s SemaphoreConnection) GetSshKeys(keyring daemon.DaemonKeyRing) ([]KeyItemResponse, error) {
	var sshKeys []KeyItemResponse
	b, err := s.Get(keyring, fmt.Sprintf("%s/%v/keys", ProjectPath, s.ProjectId))
	if err != nil {
		return sshKeys, err
	}
	err = json.Unmarshal(b, &sshKeys)
	if err != nil {
		return sshKeys, &SemaphoreClientError{Msg: err.Error()}
	}
	return sshKeys, nil
}

/*
Get SSH key by its name
*/
func (s SemaphoreConnection) GetSshKeyByName(name string, keyring daemon.DaemonKeyRing) (KeyItemResponse, error) {
	var key KeyItemResponse
	keys, err := s.GetSshKeys(keyring)
	if err != nil {
		return key, err
	}
	for i := range keys {
		if keys[i].Name == name {
			return keys[i], nil
		}
	}
	return key, &SemaphoreClientError{Msg: "Keyname not found in Semaphore key store."}

}

/*
Retrieve the SSH keys associated with a project
*/

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
