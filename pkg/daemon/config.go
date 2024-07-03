package daemon

import (
	"encoding/json"
	"log"
	"os"
)

type Configuration interface {
	Repo() string
	Branch() string
	PlaybookName() string
	Image() string
	Region() string
	LinodeType() string
}

type ConfigFromFile struct {
	Cloud   cloudConfig   `json:"cloud"`
	Ansible ansibleConfig `json:"ansible"`
}

type ansibleConfig struct {
	Repo         string `json:"repo_url"`
	Branch       string `json:"branch"`
	PlaybookName string `json:"playbook_name"`
}

func (c ConfigFromFile) Repo() string {
	return c.Ansible.Repo
}

func (c ConfigFromFile) Branch() string {
	return c.Ansible.Branch
}

func (c ConfigFromFile) PlaybookName() string { return c.Ansible.PlaybookName }

type cloudConfig struct {
	Image      string `json:"image"`
	Region     string `json:"region"`
	LinodeType string `json:"linode_type"`
}

func (c ConfigFromFile) Image() string {
	return c.Cloud.Image
}

func (c ConfigFromFile) Region() string {
	return c.Cloud.Region
}

func (c ConfigFromFile) LinodeType() string {
	return c.Cloud.LinodeType
}

func ReadConfig(path string) Configuration {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var config ConfigFromFile
	err = json.Unmarshal(b, &config)
	if err != nil {
		log.Fatal(err)
	}
	return config

}

func BlankConfig(path string) error {
	config := ConfigFromFile{
		Cloud: cloudConfig{
			Image:      "",
			Region:     "",
			LinodeType: "",
		},
		Ansible: ansibleConfig{
			Repo:   "",
			Branch: "",
		},
	}
	b, err := json.Marshal(config)
	if err != nil {
		return err
	}
	os.WriteFile(path, b, os.ModeAppend)
	return nil

}
