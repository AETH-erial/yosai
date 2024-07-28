package wg

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"text/template"
)

//go:embed wireguard.conf.templ
var confTmpl string

type WireguardTemplateSeed struct {
	VpnClientPrivateKey string
	VpnClientAddress    string
	Peers               []WireguardTemplatePeer
}

type WireguardTemplatePeer struct {
	Pubkey  string
	Address string
	Port    int
}

/*
Render out a client configuration file, utilizing the data provided from Semaphore and the daemon keyring

	    :param tmpl: a template.Template that will be populated with the VPN data
		:param wgData: a WireGuardTemplateSeed struct that contains all the info needed to populate a wireguard config file
*/
func RenderClientConfiguration(wgData WireguardTemplateSeed) ([]byte, error) {
	buff := bytes.NewBuffer([]byte{})
	tmpl, err := template.New("wireguard.conf.templ").Parse(confTmpl)
	if err != nil {
		log.Fatal(err)
	}

	err = tmpl.Execute(buff, wgData)
	if err != nil {
		return buff.Bytes(), &TemplatingError{TemplateData: wgData, Msg: err.Error()}
	}

	return buff.Bytes(), nil
}

type TemplatingError struct {
	TemplateData WireguardTemplateSeed
	Msg          string
}

func (t *TemplatingError) Error() string {
	return fmt.Sprintf("There was an error executing the template file: %s, Seed data: %+v\n", t.Msg, t.TemplateData)
}
