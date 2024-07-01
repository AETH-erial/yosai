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
	Keyring   daemon.DaemonKeyRing
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

type NewInventoryRequest struct {
	Name        string `json:"name"`
	ProjectId   int    `json:"project_id"`
	Inventory   string `json:"inventory"` // This field is where the YAML inventory file gets put, as a string (not a filepath!)
	Type        string `json:"type"`
	SshKeyId    int    `json:"ssh_key_id"`
	BecomeKeyId int    `json:"become_key_id"`
}
type NewInventoryRespone struct {
	Id          int    `json:"id"`
	Inventory   string `json:"inventory"`
	Name        string `json:"name"`
	ProjectId   int    `json:"project_id"`
	Type        string `json:"type"`
	SshKeyId    int    `json:"ssh_key_id"`
	BecomeKeyId int    `json:"become_key_id"`
}

/*
####################################################################
############ IMPLEMENTING daemon.Key FOR KeyItemResponse ###########
####################################################################
*/

func (k KeyItemResponse) GetPublic() string {
	return k.Ssh.Login
}
func (k KeyItemResponse) GetSecret() string {
	return k.Ssh.PrivateKey
}
func (k KeyItemResponse) Prepare() string {
	return k.Type
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
###################################################################
########### IMPLEMENTING THE DaemonKeyRing INTERFACE ##############
###################################################################
*/
/*
Get SSH key by its name
*/
func (s SemaphoreConnection) GetKey(name string) (daemon.Key, error) {
	var key KeyItemResponse
	keys, err := s.GetSshKeys()
	if err != nil {
		return key, err
	}
	for i := range keys {
		if keys[i].Name == name {
			return keys[i], nil
		}
	}

	return key, &KeyNotFound{Keyname: name}

}

/*
Add SSH Key to a project

	:param name: the name to assign the key in the project
	:param keyring: a daemon.DaemonKeyRing implementer that can return the API key for Semaphore
	:param key: a daemon.Key implementer wrapping the SSH key
*/
func (s SemaphoreConnection) AddKey(name string, key daemon.Key) error {
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
	_, err = s.Post(path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	return nil
}

/*
Drop a key from the Semaphore secret store
*/
func (s SemaphoreConnection) RemoveKey(name string) error {
	_, err := s.Delete(name)
	return err
}

// Return the resource name for logging purposes
func (s SemaphoreConnection) Source() string {
	return "Semaphore Keystore"
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
	semaphoreBootstrap := SemaphoreConnection{Client: client, ServerUrl: url, HttpProto: proto, Keyring: keyring}

	id, err := semaphoreBootstrap.GetProjectByName(YosaiProject)
	if err != nil {
		log.Write([]byte(YosaiProject + " NOT FOUND IN SEMAPHORE. Creating..."))
		err = semaphoreBootstrap.NewProject(YosaiProject)
		if err != nil {
			log.Write([]byte("FATAL ERROR CREATING PROJECT. ABANDONING SHIP. Error: " + err.Error()))
		}
		id, _ = semaphoreBootstrap.GetProjectByName(YosaiProject)
		log.Write([]byte("Found " + YosaiProject + " with project id: " + fmt.Sprint(id)))
		return SemaphoreConnection{
			Client:    client,
			ServerUrl: url,
			HttpProto: proto,
			ProjectId: id,
			Keyring:   keyring,
		}
	}

	return SemaphoreConnection{
		Client:    &http.Client{},
		ServerUrl: url,
		HttpProto: proto,
		ProjectId: id,
		Keyring:   keyring,
	}
}

/*
Create a new 'Project' in Semaphore

	:param name: the name to assign the project
	:param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
*/
func (s SemaphoreConnection) NewProject(name string) error {
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
	_, err = s.Post(ProjectsPath, bytes.NewReader(b))
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
func (s SemaphoreConnection) AddRepository(giturl string, branch string, id int) error {
	repoAddRequest := addRepoToProjectReq{
		Name:      fmt.Sprintf("%s:%s", giturl, branch),
		ProjectId: s.ProjectId,
		GitUrl:    giturl,
		GitBranch: branch,
		SshKeyId:  id,
	}
	b, err := json.Marshal(&repoAddRequest)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	_, err = s.Post(fmt.Sprintf("%s/%v/repositories", ProjectPath, s.ProjectId), bytes.NewReader(b))
	if err != nil {
		return err
	}
	return nil

}

/*
Generic POST Request to sent to the Semaphore server

	:param path: the path to the API to POST. Preceeding slashes will be trimmed
	:param body: an io.Reader implementer to use as the POST body. Must comply with application/json Content-Type
*/
func (s SemaphoreConnection) Post(path string, body io.Reader) ([]byte, error) {
	var b []byte
	apikey, err := s.Keyring.GetKey(keytags.SEMAPHORE_API_KEYNAME)
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

	:param path: the path to GET, added into the base API url
*/
func (s SemaphoreConnection) Get(path string) ([]byte, error) {
	var b []byte
	apiKey, err := s.Keyring.GetKey(keytags.SEMAPHORE_API_KEYNAME)
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
Generic DELETE method for calling the Semaphore server
*/
func (s SemaphoreConnection) Delete(path string) ([]byte, error) {
	return []byte{}, nil
}

/*
Retrieve the projects in Semaphore

	:param keyring: a daemon.DaemonKeyRing implementer to get the API key from for Semaphore
*/
func (s SemaphoreConnection) GetProjects() ([]ProjectsResponse, error) {
	var projectsResp []ProjectsResponse
	b, err := s.Get(ProjectsPath)
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
func (s SemaphoreConnection) GetProjectByName(name string) (int, error) {
	projects, err := s.GetProjects()
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
func (s SemaphoreConnection) GetSshKeys() ([]KeyItemResponse, error) {
	var sshKeys []KeyItemResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/keys", ProjectPath, s.ProjectId))
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
Return an SSH key ID from the Semaphore keystore by it's name

	:param keyname: the name of the key in Semaphore
*/
func (s SemaphoreConnection) GetSshKeyId(keyname string) (int, error) {
	keys, err := s.GetSshKeys()
	if err != nil {
		return 0, err
	}
	for i := range keys {
		if keys[i].Name == keyname {
			return keys[i].Id, nil
		}
	}
	return 0, &KeyNotFound{Keyname: keyname}
}

/*
######################################################
############# YAML INVENTORY STRUCTS #################
######################################################
*/
type YamlInventory struct {
	All yamlInvAll `yaml:"all"`
}

type yamlInvAll struct {
	Children yamlInvChildren `yaml:"children"`
}

type yamlInvChildren struct {
	Hosts map[string]string `yaml:"hosts"`
}

/*
YAML inventory builder function

	:param hosts: a list of host IP addresses to add to the VPN server inventory
*/
func YamlInventoryBuilder(hosts []string) YamlInventory {
	hostmap := map[string]string{}
	for i := range hosts {
		hostmap[hosts[i]] = ""
	}
	return YamlInventory{
		All: yamlInvAll{
			Children: yamlInvChildren{
				Hosts: hostmap,
			},
		},
	}

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

type KeyNotFound struct{ Keyname string }

func (k *KeyNotFound) Error() string {
	return fmt.Sprintf("Key '%s' was not found in the Semaphore Keystore", k.Keyname)
}
