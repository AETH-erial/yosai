package semaphore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	KeyTagger keytags.Keytagger
	Config    daemon.Configuration
	ServerUrl string
	HttpProto string
	ProjectId int
}

type TaskOutput struct {
	TaskID int       `json:"task_id"`
	Task   string    `json:"task"`
	Time   time.Time `json:"time"`
	Output string    `json:"output"`
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

type JobTemplate struct {
	Id            int    `json:"id"`
	ProjectId     int    `json:"project_id"`
	Name          string `json:"name"`
	InventoryId   int    `json:"inventory_id"`
	RepositoryId  int    `json:"repository_id"`
	EnvironmentId int    `json:"environment_id"`
	Playbook      string `json:"playbook"`
}

type StartTaskRequest struct {
	TemplateID int `json:"template_id"`
	ProjectId  int `json:"project_id"`
}
type StartTaskResponse struct {
	Id          int    `json:"id"`
	TemplateID  int    `json:"template_id"`
	Debug       bool   `json:"debug"`
	DryRun      bool   `json:"dry_run"`
	Diff        bool   `json:"diff"`
	Playbook    string `json:"playbook"`
	Environment string `json:"environment"`
	Limit       string `json:"limit"`
}

type AddEnvironmentRequest struct {
	Name      string `json:"name"`
	ProjectID int    `json:"project_id"`
	Password  string `json:"password"`
	JSON      string `json:"json"`
	Env       string `json:"env"`
}
type EnvironmentResponse struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	ProjectID int    `json:"project_id"`
	Password  string `json:"password"`
	JSON      string `json:"json"`
	Env       string `json:"env"`
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

func (a AddKeyRequest) GetType() string {
	return a.Type
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
func (k KeyItemResponse) GetType() string {
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
		return key, daemon.KeyRingError
	}
	for i := range keys {
		if keys[i].Name == name {
			return keys[i], nil
		}
	}

	return key, daemon.KeyNotFound

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
	fmt.Println(string(key.Prepare()))
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
func (s SemaphoreConnection) NewKeyRequestBuilder(name string, key daemon.Key) daemon.Key {
	if key.GetType() == "ssh" {
		return AddKeyRequest{
			Name:      name,
			Type:      key.GetType(),
			ProjectId: s.ProjectId,
			Ssh: sshKeyAdd{
				Login:      key.GetPublic(),
				PrivateKey: key.GetSecret(),
			},
		}
	} else {
		return AddKeyRequest{
			Name:      name,
			Type:      key.GetType(),
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
func NewSemaphoreClient(url string, proto string, log io.Writer, keyring daemon.DaemonKeyRing, conf daemon.Configuration, keytagger keytags.Keytagger) SemaphoreConnection {
	log.Write([]byte("Using HTTP mode: " + proto + "\n"))
	client := &http.Client{}
	semaphoreBootstrap := SemaphoreConnection{Client: client, ServerUrl: url, HttpProto: proto, Keyring: keyring, KeyTagger: keytagger}

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
			Config:    conf,
			KeyTagger: keytagger,
		}
	}

	return SemaphoreConnection{
		Client:    &http.Client{},
		ServerUrl: url,
		HttpProto: proto,
		ProjectId: id,
		Keyring:   keyring,
		Config:    conf,
		KeyTagger: keytagger,
	}
}

/*
Create a new 'Project' in Semaphore

	:param name: the name to assign the project
	:param keyring: a daemon.DaemonKeyRing implementer to get the Semaphore API key from
*/
func (s SemaphoreConnection) NewProject(name string) error {
	_, err := s.GetProjectByName(name)
	if err == nil {
		return nil // return nil of project already exists
	}
	var b []byte
	newProj := NewProjectReqeust{
		Name:             name,
		Alert:            false,
		AlertChat:        "",
		MaxParallelTasks: 0,
	}
	b, err = json.Marshal(&newProj)
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
	sshKeyId, err := s.GetKeyId(s.KeyTagger.GitSshKeyname())
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
func (s SemaphoreConnection) Put(path string, body io.Reader) ([]byte, error) {
	var b []byte
	apikey, err := s.Keyring.GetKey(s.KeyTagger.SemaphoreApiKeyname())
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s://%s/%s", s.HttpProto, s.ServerUrl, strings.TrimPrefix(path, "/")), body)
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
Generic POST Request to sent to the Semaphore server

	:param path: the path to the API to POST. Preceeding slashes will be trimmed
	:param body: an io.Reader implementer to use as the POST body. Must comply with application/json Content-Type
*/
func (s SemaphoreConnection) Post(path string, body io.Reader) ([]byte, error) {
	var b []byte
	apikey, err := s.Keyring.GetKey(s.KeyTagger.SemaphoreApiKeyname())
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
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return b, &SemaphoreClientError{Msg: err.Error()}
	}
	if resp.StatusCode >= 400 {
		fmt.Println(string(b))
		return b, &SemaphoreClientError{Msg: resp.Status}
	}
	return b, nil

}

/*
Agnostic GET method for calling the upstream Semaphore server

	:param path: the path to GET, added into the base API url
*/
func (s SemaphoreConnection) Get(path string) ([]byte, error) {
	var b []byte
	apiKey, err := s.Keyring.GetKey(s.KeyTagger.SemaphoreApiKeyname())
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
Get the output of a task

	:param taskId: the ID of the task that was ran
*/
func (s SemaphoreConnection) GetTaskOutput(taskId int) ([]TaskOutput, error) {
	var taskout []TaskOutput
	b, err := s.Get(fmt.Sprintf("%s/%v/tasks/%v/output", ProjectPath, s.ProjectId, taskId))
	if err != nil {
		return taskout, err
	}
	err = json.Unmarshal(b, &taskout)
	if err != nil {
		return taskout, &SemaphoreClientError{Msg: "Could not unmarshall the response from getting task output." + err.Error()}
	}
	return taskout, nil

}

/*
Add an inventory to semaphore

	:param hosts: a list of IP addresses to add to the inventory
*/
func (s SemaphoreConnection) AddInventory(name string, hosts ...string) error {
	_, err := s.GetInventoryByName(name)
	if err == nil { // Returning on nil error because that means the inventory exists
		return &SemaphoreClientError{Msg: "Inventory Exists! Please update rather than create a new."}
	}
	sshKeyId, err := s.GetKeyId(s.KeyTagger.VpsSvcAccSshPubkeySeed())
	if err != nil {
		return err
	}
	becomeKeyId, err := s.GetKeyId(s.KeyTagger.VpsSvcAccKeyname())
	if err != nil {
		return err
	}
	pubkey, err := s.Keyring.GetKey(s.KeyTagger.WgClientKeypairKeyname())
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error() + s.KeyTagger.WgClientKeypairKeyname()}
	}
	inv := s.YamlInventoryBuilder(hosts, pubkey.GetPublic())
	b, err := yaml.Marshal(inv)
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error()}
	}
	body := NewInventoryRequest{
		Name:        name,
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
func (s SemaphoreConnection) GetInventoryByName(name string) (InventoryResponse, error) {
	var out InventoryResponse
	resp, err := s.GetAllInventories()
	if err != nil {
		return out, err
	}
	for i := range resp {
		if resp[i].Name == name {
			return resp[i], nil
		}
	}
	return out, &KeyNotFound{Keyname: name}

}

/*
Get all inventories from Semaphore
*/
func (s SemaphoreConnection) GetAllInventories() ([]InventoryResponse, error) {
	var resp []InventoryResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/%s", ProjectPath, s.ProjectId, "inventory"))
	if err != nil {
		return resp, err

	}
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return resp, &SemaphoreClientError{Msg: err.Error()}
	}
	return resp, nil
}

/*
Update an inventory
*/
func (s SemaphoreConnection) UpdateInventory(name string, inv YamlInventory) error {
	sshKeyId, err := s.GetKeyId(s.KeyTagger.VpsSvcAccSshPubkeySeed())
	if err != nil {
		return err
	}
	becomeKeyId, err := s.GetKeyId(s.KeyTagger.VpsSvcAccKeyname())
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(inv)
	if err != nil {
		return &SemaphoreClientError{Msg: "Error unmarshalling YAML inventory payload: " + err.Error()}
	}
	targetInv, err := s.GetInventoryByName(name)
	if err != nil {
		return &SemaphoreClientError{Msg: "Target inventory: " + name + " was not found."}
	}
	body := InventoryResponse{
		Id:          targetInv.Id,
		Name:        name,
		ProjectId:   s.ProjectId,
		Inventory:   string(b),
		SshKeyId:    sshKeyId,
		BecomeKeyId: becomeKeyId,
		Type:        "static-yaml",
	}
	req, err := json.Marshal(body)
	if err != nil {
		return &SemaphoreClientError{Msg: "There was an error marshalling the JSON payload: " + err.Error()}
	}
	_, err = s.Put(fmt.Sprintf("%s/%v/inventory/%v", ProjectPath, s.ProjectId, targetInv.Id), bytes.NewReader(req))
	return err

}

/*
Remove host from an inventory
*/
func (s SemaphoreConnection) RemoveHostFromInv(name string, host ...string) error {
	resp, err := s.GetInventoryByName(name)
	if err != nil {
		return err
	}
	var inv YamlInventory
	err = yaml.Unmarshal([]byte(resp.Inventory), &inv)
	if err != nil {
		return &SemaphoreClientError{Msg: "Error unmarshalling inventory from server: " + resp.Inventory + err.Error()}
	}
	for i := range host {
		_, ok := inv.All.Hosts[host[i]]
		if !ok {
			return &SemaphoreClientError{Msg: "Host: " + host[i] + " not found in the inventory: " + resp.Inventory}
		}
		delete(inv.All.Hosts, host[i])
	}
	pubkey, err := s.Keyring.GetKey(s.KeyTagger.WgClientKeypairKeyname())
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error() + s.KeyTagger.WgClientKeypairKeyname()}
	}
	var hosts []string
	for k := range inv.All.Hosts {
		hosts = append(hosts, k)
	}

	return s.UpdateInventory(name, s.YamlInventoryBuilder(hosts, pubkey.GetPublic()))

}

/*
Add hosts to inventory
*/
func (s SemaphoreConnection) AddHostToInv(name string, host ...string) error {

	resp, err := s.GetInventoryByName(name)
	if err != nil {
		return err
	}
	var inv YamlInventory
	err = yaml.Unmarshal([]byte(resp.Inventory), &inv)
	if err != nil {
		return &SemaphoreClientError{Msg: "Error unmarshalling inventory from server: " + resp.Inventory + err.Error()}
	}
	pubkey, err := s.Keyring.GetKey(s.KeyTagger.WgClientKeypairKeyname())
	if err != nil {
		return &SemaphoreClientError{Msg: err.Error() + s.KeyTagger.WgClientKeypairKeyname()}
	}

	var hosts []string
	for k := range inv.All.Hosts {
		hosts = append(hosts, k)
	}
	hosts = append(hosts, host...)
	return s.UpdateInventory(name, s.YamlInventoryBuilder(hosts, pubkey.GetPublic()))
}

/*
Get a repo ID by its name
:param name: the name of the repo
*/
func (s SemaphoreConnection) GetRepoByName(name string) (int, error) {
	resp, err := s.GetAllRepos()
	if err != nil {
		return 0, err
	}
	for i := range resp {
		if resp[i].Name == name {
			return resp[i].Id, nil
		}
	}

	return 0, &KeyNotFound{Keyname: name}
}

/*
Get all repositories from Semaphore
*/
func (s SemaphoreConnection) GetAllRepos() ([]NewRepoResponse, error) {
	var resp []NewRepoResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/%s", ProjectPath, s.ProjectId, "repositories"))
	if err != nil {
		return resp, &SemaphoreClientError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return resp, &SemaphoreClientError{Msg: err.Error()}
	}
	return resp, nil

}

// Create an environment variable configuration, currently unimplemented
func (s SemaphoreConnection) AddEnvironment() error {
	_, err := s.GetEnvironmentId(YosaiEnvironment)
	if err == nil {
		return nil // environment exists, dont add another with same name
	}
	var body AddEnvironmentRequest
	body = AddEnvironmentRequest{
		Name:      YosaiEnvironment,
		ProjectID: s.ProjectId,
		JSON:      "{}",
		Env:       "{}",
	}
	b, err := json.Marshal(body)
	if err != nil {
		return &SemaphoreClientError{Msg: "couldnt marshal the JSON payload"}
	}
	_, err = s.Post(fmt.Sprintf("%s/%v/environment", ProjectPath, s.ProjectId), bytes.NewBuffer(b))
	return err

}

// Get an environment configuration ID by name.
func (s SemaphoreConnection) GetEnvironmentId(name string) (int, error) {
	var env []EnvironmentResponse
	b, err := s.Get(fmt.Sprintf("%s/%v/environment", ProjectPath, s.ProjectId))
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(b, &env)
	if err != nil {
		return 0, &SemaphoreClientError{Msg: "Couldnt unmarshall the response"}
	}
	for i := range env {
		if env[i].Name == name {
			return env[i].Id, nil
		}
	}
	return 0, &KeyNotFound{Keyname: "Couldnt find environment: " + name}
}

/*
Add job template to the Yosai project on Semaphore
:param playbook: the name of the playbook file
:param repoName: the name of the repo that the playbook belongs to
*/
func (s SemaphoreConnection) AddJobTemplate(playbook string, repoName string) error {
	_, err := s.JobTemplateByName(YosaiVpnRotationJob)
	if err == nil {
		return nil // return nil because template exists

	}
	repoId, err := s.GetRepoByName(repoName)
	if err != nil {
		return err
	}
	InventoryItem, err := s.GetInventoryByName(YosaiServerInventory)
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
		InventoryId:   InventoryItem.Id,
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
Start a task in Semaphore by the template name

	:param name: the name of the job template to start
*/
func (s SemaphoreConnection) StartJob(name string) (StartTaskResponse, error) {
	var resp StartTaskResponse
	template, err := s.JobTemplateByName(name)
	if err != nil {
		return resp, &SemaphoreClientError{Msg: "Could not start job template: " + name + "Error: " + err.Error()}
	}
	var jobReq StartTaskRequest
	jobReq = StartTaskRequest{
		TemplateID: template.Id,
		ProjectId:  s.ProjectId,
	}
	b, err := json.Marshal(&jobReq)
	if err != nil {
		return resp, &SemaphoreClientError{Msg: "Couldnt marshal data into byte array: " + err.Error()}
	}
	rb, err := s.Post(fmt.Sprintf("%s/%v/tasks", ProjectPath, s.ProjectId), bytes.NewReader(b))
	if err != nil {
		return resp, err
	}
	err = json.Unmarshal(rb, &resp)
	if err != nil {
		return resp, &SemaphoreClientError{Msg: "Couldnt unmarshal the response from semaphore: " + err.Error()}
	}
	return resp, nil

}

/*
Get a job template ID by name

	:param name: the name of the job template ID
*/
func (s SemaphoreConnection) GetAllTemplates() ([]JobTemplate, error) {
	var jobs []JobTemplate
	resp, err := s.Get(fmt.Sprintf("%s/%v/templates", ProjectPath, s.ProjectId))
	if err != nil {
		return jobs, err
	}
	err = json.Unmarshal(resp, &jobs)
	if err != nil {
		return jobs, &SemaphoreClientError{Msg: "Error unmarshalling payload response: " + err.Error()}
	}
	return jobs, nil

}

/*
Bootstrap the Semaphore environment
*/

/*
Get a job template ID by name

	:param name: the name of the job template ID
*/
func (s SemaphoreConnection) JobTemplateByName(name string) (JobTemplate, error) {
	var job JobTemplate
	jobs, err := s.GetAllTemplates()
	if err != nil {
		return job, err
	}

	for i := range jobs {
		if jobs[i].Name == name {
			return jobs[i], nil
		}
	}
	return job, &SemaphoreClientError{Msg: "Job with name" + name + "not found"}
}

/*
##########################################################
################## DAEMON ROUTE HANDLERS #################
##########################################################
*/

type SemaphoreRequest struct {
	Target string `json:"target"`
}

/*
Wrapping the functioanlity of the keyring bootstrapper for top level cleanliness
*/
func (s SemaphoreConnection) keyBootstrapper() daemon.SockMessage {
	reqKeys := s.KeyTagger.GetAnsibleKeys()
	for i := range reqKeys {
		kn := reqKeys[i]
		key, err := s.Keyring.GetKey(kn)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		err = s.AddKey(kn, s.NewKeyRequestBuilder(kn, key))
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
	}
	return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Daemon keyring successfuly bootstrapped."))
}

/*
Wrapping the functionality of the Project bootstrapper for top level cleanliness
*/
func (s SemaphoreConnection) projectBootstrapper() daemon.SockMessage {
	err := s.NewProject(YosaiProject)
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	err = s.AddRepository(s.Config.Repo(), s.Config.Branch())
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	err = s.AddEnvironment()
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	err = s.AddJobTemplate(s.Config.PlaybookName(), fmt.Sprintf("%s:%s", s.Config.Repo(), s.Config.Branch()))
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Project successfuly bootstrapped."))

}

/*
Wrapping the inventory bootstrap functionality for top level cleanliness
*/
func (s SemaphoreConnection) inventoryBootstrapper() daemon.SockMessage {
	err := s.AddInventory(YosaiServerInventory, s.Config.VpnServer())
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	err = s.AddHostToInv(YosaiServerInventory, s.Config.VpnServer())
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Inventory successfuly bootstrapped."))

}

func (s SemaphoreConnection) BootstrapHandler(msg daemon.SockMessage) daemon.SockMessage {
	var req SemaphoreRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	switch req.Target {
	case "keys":
		return s.keyBootstrapper()
	case "project":
		return s.projectBootstrapper()
	case "inventory":
		return s.inventoryBootstrapper()
	default:
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Unresolved Method."))

	}

}

/*
Router for handling all stuff relating to Projects

	:param msg: a daemon.SockMessage with request info
*/
func (s SemaphoreConnection) projectHandler(msg daemon.SockMessage) daemon.SockMessage {
	switch msg.Method {
	case "bootstrap":
		return s.projectBootstrapper()
	case "add":
		var req SemaphoreRequest
		err := json.Unmarshal(msg.Body, &req)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		err = s.NewProject(req.Target)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Project: "+req.Target+" successfully added."))
	case "show":
		proj, err := s.GetProjects()
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		b, err := json.MarshalIndent(proj, " ", "    ")
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgResponse, b)
	default:
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Unresolved Method."))

	}

}

/*
handler to wrap all functions relating to Tasks

	:param msg: a daemon.SockMessage that contains the request information
*/
func (s SemaphoreConnection) taskHandler(msg daemon.SockMessage) daemon.SockMessage {
	var req SemaphoreRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	switch msg.Method {
	case "run":
		resp, err := s.StartJob(req.Target)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		b, err := json.MarshalIndent(resp, " ", "    ")
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgResponse, b)
	case "show":
		taskid, err := strconv.Atoi(req.Target)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		taskout, err := s.GetTaskOutput(taskid)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		b, err := json.MarshalIndent(taskout, " ", "    ")
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgResponse, b)
	default:
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte("Unresolved Method."))

	}
}

/*
Handles all of the requests relating to Hosts
    :param msg: a daemon.SockMessage containing all of the request info
*/
func (s SemaphoreConnection) hostHandler(msg daemon.SockMessage) daemon.SockMessage {
	var req SemaphoreRequest
	err := json.Unmarshal(msg.Body, &req)
	if err != nil {
		return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
	}
	switch msg.Method {
	case "add":
		hosts := strings.Split(strings.Trim(req.Target, ","), ",")
		err := s.AddHostToInv(YosaiServerInventory, hosts...)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgRequest, []byte("Host: " + hosts + " added to the inventory")) 

	case "delete":
		hosts := strings.Split(strings.Trim(req.Target, ","), ",")
		err := s.RemoveHostFromInv(YosaiServerInventory, hosts...)
		if err != nil {
			return *daemon.NewSockMessage(daemon.MsgResponse, []byte(err.Error()))
		}
		return *daemon.NewSockMessage(daemon.MsgRequest, []byte("Host: " + hosts + " removed from the inventory")) 
		}
}

/*
Implementing the router interface
*/
func (s SemaphoreConnection) SemaphoreRouter(msg daemon.SockMessage) daemon.SockMessage {
	switch msg.Target {
	case "bootstrap":
		return s.BootstrapHandler(msg)
	case "project":
		return s.projectHandler(msg)
	case "task":
		return s.taskHandler(msg)
	case "hosts":
		return s.hostHandler(msg)

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
	MachineType          string `yaml:"machine_type"`
	MachineSubType       string `yaml:"machine_subtype"`
	VpnNetworkAddress    string `yaml:"vpn_network_address"`
	VpnServerPort        int    `yaml:"vpn_server_port"`
	ClientPubkey         string `yaml:"client_public_key"`
	ClientVpnAddress     string `yaml:"client_vpn_address"`
	SecretsProvider      string `yaml:"secrets_provider"`
}

/*
YAML inventory builder function

	:param hosts: a list of host IP addresses to add to the VPN server inventory
*/
func (s SemaphoreConnection) YamlInventoryBuilder(hosts []string, clientPubkey string) YamlInventory {

	hostmap := map[string]yamlVars{}
	for i := range hosts {
		hostmap[hosts[i]] = yamlVars{
			AnsibleSshCommonArgs: "-o StrictHostKeyChecking=no",
			MachineType:          "vpn",
			MachineSubType:       "server",
			VpnNetworkAddress:    s.Config.VpnServerNetwork(),
			VpnServerPort:        53280,
			ClientPubkey:         clientPubkey,
			ClientVpnAddress:     s.Config.VpnClientIpAddr(),
			SecretsProvider:      "hashicorp"}
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
