package config

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
)

/*
Run a new webserver

	:param port: port number to run the webserver on
*/
func RunHttpServer(port int, dbhook DatabaseIO) {
	execHndl := &ExecutionHandler{DbHook: dbhook}
	http.Handle("/refresh", execHndl)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", port), nil))
}

type ExecutionHandler struct {
	DbHook DatabaseIO
}

/*
Handler for the route to retrieve a user configuration

	    :param w: the http.ResponseWriter to write the response into
		:param req: a pointer to the http.Request to parse
*/
func (e *ExecutionHandler) GetUserConfiguration(w http.ResponseWriter, req *http.Request) {
	params, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	return

}
