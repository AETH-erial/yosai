package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
)

const UserQueryParam = "username"

/*
Run a new webserver

	:param port: port number to run the webserver on
*/
func RunHttpServer(port int, dbhook DatabaseIO, loggingOut io.Writer) {
	execHndl := &ExecutionHandler{DbHook: dbhook, out: loggingOut}
	http.HandleFunc("/get-config/{username}", execHndl.GetUserConfiguration)
	http.HandleFunc("/update-config/{username}", execHndl.UpdateUserConfiguration)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", port), nil))
}

type ExecutionHandler struct {
	DbHook DatabaseIO
	out    io.Writer
}

func (e *ExecutionHandler) Log(msg ...string) {
	logMsg := "YOSAI SERVER LOGGER ::: "
	for i := range msg {
		logMsg = logMsg + " " + msg[i] + "\n"
	}
	e.out.Write([]byte(logMsg))
}

/*
Handler for the route to retrieve a user configuration

	    :param w: the http.ResponseWriter to write the response into
		:param req: a pointer to the http.Request to parse
*/
func (e *ExecutionHandler) GetUserConfiguration(w http.ResponseWriter, req *http.Request) {
	e.Log("Recieved request: ", req.URL.Path)
	if req.Method != http.MethodGet {
		e.Log("Unsupported method: ", req.Method, "to endpoint: ", req.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user := req.PathValue(UserQueryParam)
	if user == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	e.Log("Called from: ", user)
	config, err := e.DbHook.GetConfigByUser(ValidateUsername(user))
	if err != nil {
		e.Log(err.Error())
		w.WriteHeader(http.StatusNotFound)
		return
	}
	e.Log("Config gotten successfully.")
	b, err := json.Marshal(config)
	if err != nil {
		e.Log(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(b)
	if err != nil {
		e.Log(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	return

}

/*
Handler for updating the calling users configuration

	    :param w: the http.ResponseWriter to write the response into
		:param req: a pointer to the http.Request to parse
*/
func (e *ExecutionHandler) UpdateUserConfiguration(w http.ResponseWriter, req *http.Request) {
	e.Log("Recieved request: ", req.URL.Path)
	if req.Method != http.MethodPost || req.Method != http.MethodPatch {
		e.Log("Unsupported method: ", req.Method, "to endpoint: ", req.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user := req.PathValue(UserQueryParam)
	if user == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	e.Log("Called from: ", user)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		e.Log(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var config daemon.Configuration
	if err = json.Unmarshal(body, &config); err != nil {
		e.Log(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err = e.DbHook.UpdateUser(ValidateUsername(user), config); err != nil {
		e.Log(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	return

}
