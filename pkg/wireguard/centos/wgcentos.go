package wg

import (
	"bytes"
	"fmt"
	"text/template"
)

type WireguardTemplateSeed struct {
	VpnClientPrivateKey string
	VpnClientAddress    string
	Peers               []wireguardTemplatePeer
}

type wireguardTemplatePeer struct {
	Pubkey  string
	Address string
	Port    string
}

/*
Render out a client configuration file, utilizing the data provided from Semaphore and the daemon keyring

	    :param tmpl: a template.Template that will be populated with the VPN data
		:param wgData: a WireGuardTemplateSeed struct that contains all the info needed to populate a wireguard config file
*/
func RenderClientConfiguration(tmpl template.Template, wgData WireguardTemplateSeed) ([]byte, error) {
	var b []byte
	buff := bytes.NewBuffer(b)
	err := tmpl.Execute(buff, wgData)
	if err != nil {
		return b, &TemplatingError{TemplateData: wgData, Msg: err.Error()}
	}

	return b, nil
}

type TemplatingError struct {
	TemplateData WireguardTemplateSeed
	Msg          string
}

func (t *TemplatingError) Error() string {
	return fmt.Sprintf("There was an error executing the template file: %s, Seed data: %+v\n", t.Msg, t.TemplateData)
}
