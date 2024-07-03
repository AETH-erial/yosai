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
	"gopkg.in/yaml.v3"
)

const ProjectsPath = "api/projects"
const ProjectPath = "api/project"
const YosaiProject = "Yosai VPN Sentinel"
const YosaiServerInventory = "Yosai VPN Servers"
const YosaiVpnRotationJob = "VPN Rotation playbook"
const YosaiEnvironment = "VPN Server configuration environment variables"

type SemaphoreConnection struct {
	Client    *http.Client
	Keyring   daemon.DaemonKeyRing
	ServerUrl string
	HttpProto string
	ProjectId int
}

type NewTemplateRequest struct {
	ProjectId     int    `json:"project_id"`
	Name          string `json:"name"`
	InventoryId   int    `json:"inventory_id"`
	RepositoryId  int    `json:"repository_id"`
	EnvironmentId int    `json:"environment_id"`
	Playbook      string `json:"playbook"`
	Type          string `json:"type"`
}

type ProjectsResponse struct {
	Id               int    `json:"id"`
	Name             string `json:"name"`
	Created          string `json:"created"`
	Alert            bool   `json:"alert"`
	AlertChat        string `json:"alert_chat"`
	MaxParallelTasks int    `json:"max_parallel_tasks"`
}

type NewProjectReqeust struct {
	Name             string `json:"name"`
	Alert            bool   `json:"alert"`
	AlertChat        string `json:"alert_chat"`
	MaxParallelTasks int    `json:"max_parallel_tasks"`
}

type NewRepoRequest struct {
	Name      string `json:"name"`       // name of the project
	ProjectId int    `json:"project_id"` // the numerical ID of the project as per /api/project/<project id>
	GitUrl    string `json:"git_url"`    // the URL of the git repo (SSH address)
	GitBranch string `json:"git_branch"` // the branch to pull down
	SshKeyId  int    `json:"ssh_key_id"` // the numerical ID of the ssh key for the repository, as per /api/project/<project id>/keys
}

type NewRepoResponse struct {
	Id        int    `json:"id"`         // the numerical ID assigned to the repo by Semaphore
	Name      string `json:"name"`       // name of the project
	ProjectId int    `json:"project_id"` // the numerical ID of the project as per /api/project/<project id>
	GitUrl    string `json:"git_url"`    // the URL of the git repo (SSH address)
	GitBranch string `json:"git_branch"` // the branch to pull down
	SshKeyId  int    `json:"ssh_key_id"` // the numerical ID of the ssh key for the repository, as per /api/project/<project id>/keys
}

type AddKeyRequest struct {
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	ProjectId     int           `json:"project_id"`
	LoginPassword loginPassword `json:"login_password"`
	Ssh           sshKeyAdd     `json:"ssh"`
}

func (a AddKeyRequest) GetPublic() string {
	if a.Type == "ssh" {
		return a.Ssh.Login
	} else {
		return a.LoginPassword.Login
	}
}

func (a AddKeyRequest) GetSecret() string {
	if a.Type == "ssh" {
		return a.Ssh.PrivateKey
	} else {
		return a.LoginPassword.Password
	}
}

func (a AddKeyRequest) Prepare() string {
	b, err := json.Marshal(a)
	if err != nil {
		return err.Error()
	}
	return string(b)
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
type InventoryResponse struct {
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
	keys, err := s.GetAllKeys()
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
	_, err := s.GetKeyId(name)
	if err == nil { // return if the key exists
		return nil
	}
	path := fmt.Sprintf("%s/%v/keys", ProjectPath, s.ProjectId)
	_, err = s.Post(path, bytes.NewReader([]byte(key.Prepare())))
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

// NewKeyRequest builder function
func (s SemaphoreConnection) NewKeyRequestBuilder(name string, keytype string, key daemon.Key) daemon.Key {
	if keytype == "ssh" {
		return AddKeyRequest{
			Name:      name,
			Type:      keytype,
			ProjectId: s.ProjectId,
			Ssh: sshKeyAdd{
				Login:      key.GetPublic(),
				PrivateKey: key.GetSecret(),
			},
		}
	} else {
		return AddKeyRequest{
			Name:      name,
			Type:      keytype,
			ProjectId: s.ProjectId,
			LoginPassword: loginPassword{
				Login:    key.GetPublic(),
				Password: key.GetSecret(),
			},
		}
	}
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
	newProj := NewProjectReqeust{
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
*/
func (s SemaphoreConnection) AddRepository(giturl string, branch string) error {
	_, err := s.GetRepoByName(fmt.Sprintf("%s:%s", giturl, branch))
	if err == nil { // return if the repo exists
		return nil
	}
	sshKeyId, err := s.GetKeyId(keytags.GIT_SSH_KEYNAME)
	if err != nil {
		return err
	}
	repoAddRequest := NewRepoRequest{
		Name:      fmt.Sprintf("%s:%s", giturl, branch),
		ProjectId: s.ProjectId,
		GitUrl:    giturl,
		GitBranch: branch,
		SshKeyId:  sshKeyId,
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
	fmt.Printf("called from inside semaphore: %s", apikey.GetSecret())
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
func (s SemaphoreConnection) GetAllKeys() ([]KeyItemResponse, error) {
	var keys []KeyItemResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/keys", ProjectPath, s.ProjectId))
	if err != nil {
		return keys, err
	}
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return keys, &SemaphoreClientError{Msg: err.Error()}
	}
	return keys, nil
}

/*
Return a key ID from the Semaphore keystore by it's name

	:param keyname: the name of the key in Semaphore
*/
func (s SemaphoreConnection) GetKeyId(keyname string) (int, error) {
	keys, err := s.GetAllKeys()
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
Add an inventory to semaphore

	:param hosts: a list of IP addresses to add to the inventory
*/
func (s SemaphoreConnection) AddInventory(hosts []string) error {
	sshKeyId, err := s.GetKeyId(keytags.VPS_SSH_KEY_KEYNAME)
	if err != nil {
		return err
	}
	becomeKeyId, err := s.GetKeyId(keytags.VPS_SUDO_USER_KEYNAME)
	if err != nil {
		return err
	}
	inv := YamlInventoryBuilder(hosts)
	b, err := yaml.Marshal(inv)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	body := NewInventoryRequest{
		Name:        YosaiServerInventory,
		ProjectId:   s.ProjectId,
		Inventory:   string(b),
		SshKeyId:    sshKeyId,
		BecomeKeyId: becomeKeyId,
		Type:        "static-yaml",
	}
	requestBody, err := json.Marshal(&body)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	_, err = s.Post(fmt.Sprintf("%s/%v/%s", ProjectPath, s.ProjectId, "inventory"), bytes.NewReader(requestBody))
	return err
}

/*
Get Inventory by name and return its ID
:param name: the name of the inventory to find
*/
func (s SemaphoreConnection) GetInventoryId(name string) (int, error) {
	var resp []InventoryResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/%s", ProjectPath, s.ProjectId, "inventory"))
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return 0, &SemaphoreClientError{Msg: err.Error()}
	}
	for i := range resp {
		if resp[i].Name == name {
			return resp[i].Id, nil
		}
	}
	return 0, &KeyNotFound{Keyname: name}

}

/*
Get a repo ID by its name
:param name: the name of the repo
*/
func (s SemaphoreConnection) GetRepoByName(name string) (int, error) {
	var resp []NewRepoResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/%s", ProjectPath, s.ProjectId, "repositories"))
	if err != nil {

		return 0, &SemaphoreClientError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, &resp)
	if err != nil {

		return 0, &SemaphoreClientError{Msg: err.Error()}
	}
	for i := range resp {
		if resp[i].Name == name {
			return resp[i].Id, nil
		}
	}

	return 0, &KeyNotFound{Keyname: name}
}

// Create an environment variable configuration, currently unimplemented
func (s SemaphoreConnection) AddEnvironment(vars map[string]interface{}) error {
	return nil
}

// Get an environment configuration ID by name. Currently statically return '8' as its the environment I created in semaphore
func (s SemaphoreConnection) GetEnvironmentId(name string) (int, error) {
	return 8, nil
}

/*
Add job template to the Yosai project on Semaphore
:param playbook: the name of the playbook file
:param repoName: the name of the repo that the playbook belongs to
*/
func (s SemaphoreConnection) AddJobTemplate(playbook string, repoName string) error {
	repoId, err := s.GetRepoByName(repoName)
	if err != nil {
		return err
	}
	InventoryId, err := s.GetInventoryId(YosaiServerInventory)
	if err != nil {
		return err
	}
	envId, err := s.GetEnvironmentId(YosaiEnvironment)
	if err != nil {
		return err
	}
	templ := NewTemplateRequest{
		ProjectId:     s.ProjectId,
		Name:          YosaiVpnRotationJob,
		InventoryId:   InventoryId,
		RepositoryId:  repoId,
		EnvironmentId: envId,
		Playbook:      playbook,
		Type:          "",
	}
	b, err := json.Marshal(templ)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	b, err = s.Post(fmt.Sprintf("%s/%v/%s", ProjectPath, s.ProjectId, "templates"), bytes.NewReader(b))
	if err != nil {
		return &SemaphoreClientError{Msg: fmt.Sprintf("Error: %s\nServer Response: %s", err.Error(), string(b))}
	}
	return nil

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
	Hosts map[string]yamlVars `yaml:"hosts"`
}

type yamlVars struct {
	AnsibleSshCommonArgs string `yaml:"ansible_ssh_common_args"`
}

/*
YAML inventory builder function

	:param hosts: a list of host IP addresses to add to the VPN server inventory
*/
func YamlInventoryBuilder(hosts []string) YamlInventory {

	hostmap := map[string]yamlVars{}
	for i := range hosts {
		hostmap[hosts[i]] = yamlVars{AnsibleSshCommonArgs: "-o StrictHostKeyChecking=no"}
	}
	return YamlInventory{
		All: yamlInvAll{
			Hosts: hostmap,
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
