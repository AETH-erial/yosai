package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
)

const UNIX_DOMAIN_SOCK_PATH = "/tmp/yosaid.sock"

func reader(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf[:])
		if err != nil {
			return
		}
		println("Client got:", string(buf[0:n]))
	}
}
func main() {
	if len(os.Args) < 4 {
		log.Fatal("Not enough arguments!")
	}
	var args []string
	args = os.Args[1:]
	msgPack := daemon.NewSockMessage(args[0], args[1], args[2])
	data := daemon.Marshal(msgPack)

	//conf := daemon.ReadConfig("./.config.json")

	conn, err := net.Dial("unix", UNIX_DOMAIN_SOCK_PATH)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	buf := bytes.NewBuffer([]byte(data))
	_, err = io.Copy(conn, buf)
	if err != nil {
		log.Fatal("write error:", err)
	}
	var b []byte
	resp := bytes.NewBuffer(b)
	_, err = io.Copy(resp, conn)
	if err != nil {
		if err == io.EOF {
			fmt.Println("exited ok.")
			os.Exit(0)
		}
		log.Fatal(err)
	}
	respbytes, err := io.ReadAll(resp)
	fmt.Println(string(respbytes), err)

	/*
	   	if os.Args[1] == Config {
	   		method := os.Args[2]
	   		switch method {
	   		case "init":
	   			daemon.BlankConfig("./.config.template.json")
	   			os.Exit(0)
	   		}
	   	}

	   	if os.Args[1] == Bootstrap {
	   		semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
	   		apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)

	   		linodeReq, err := linode.NewLinodeBodyBuilder(conf.Image(), conf.Region(), conf.LinodeType(), apikeyring)
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		fmt.Printf("%+v\n", linodeReq)
	   		newLnResp, err := lnclient.CreateNewLinode(apikeyring, linodeReq)
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		// Seed Semaphore with details
	   		gitKey, err := apikeyring.GetKey(keytags.GIT_SSH_KEYNAME)
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		keyReq := semaphoreConn.NewKeyRequestBuilder(keytags.GIT_SSH_KEYNAME, "ssh", gitKey)
	   		err = semaphoreConn.AddKey(keytags.GIT_SSH_KEYNAME, keyReq)
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		err = semaphoreConn.AddRepository(conf.Repo(), conf.Branch())
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		err = semaphoreConn.AddInventory(newLnResp.Ipv4)
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		err = semaphoreConn.AddJobTemplate("playbook_gather_facts.yml", fmt.Sprintf("%s:%s", conf.Repo(), conf.Branch()))
	   		if err != nil {
	   			log.Fatal(err)
	   		}
	   		os.Exit(0)

	   }

	   	if os.Args[1] == Key {
	   		method := os.Args[2]
	   		switch method {
	   		case "tags":
	   			for k := range keytags.AllTags {
	   				fmt.Println(k)
	   			}
	   			os.Exit(0)
	   		case "show":
	   			found, err := apikeyring.GetKey(os.Args[3])
	   			if err != nil {
	   				log.Fatal("Key '", os.Args[3], "' Not found.")
	   			}
	   			fmt.Printf("%+v\n", found)
	   			os.Exit(0)
	   		case "add":
	   			semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
	   			apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)
	   			sshkey, err := apikeyring.GetKey(keytags.GIT_SSH_KEYNAME)
	   			if err != nil {
	   				log.Fatal(err)
	   			}
	   			err = semaphoreConn.AddKey(keytags.GIT_SSH_KEYNAME, daemon.SshKey{User: sshkey.GetPublic(), PrivateKey: sshkey.GetSecret()})
	   			if err != nil {
	   				log.Fatal(err)
	   			}

	   		}
	   	}

	   	if os.Args[1] == Sem {
	   		semaphoreConn := semaphore.NewSemaphoreClient(os.Getenv("SEMAPHORE_SERVER_URL"), "https", os.Stdout, apikeyring)
	   		apikeyring.Rungs = append(apikeyring.Rungs, semaphoreConn)
	   		method := os.Args[2]
	   		switch method {
	   		case "show":
	   			proj, err := semaphoreConn.GetProjects()
	   			if err != nil {
	   				log.Fatal("Error creating a new semaphore project: ", err)
	   			}
	   			fmt.Printf("%+v\n", proj)
	   			os.Exit(0)
	   		case "status":
	   			fmt.Printf("Yosai Server ID: %v\nSemaphore Upstream: %s\n", semaphoreConn.ProjectId, semaphoreConn.ServerUrl)
	   			os.Exit(0)
	   		case "add":
	   			err = semaphoreConn.AddJobTemplate(conf.PlaybookName(), fmt.Sprintf("%s:%s", conf.Repo(), conf.Branch()))
	   			if err != nil {
	   				log.Fatal(err)
	   			}
	   			os.Exit(0)
	   		case "inv":
	   			err = semaphoreConn.AddInventory([]string{os.Args[3]})
	   			if err != nil {
	   				log.Fatal(err)
	   			}
	   		}

	   }

	   	if os.Args[1] == Cloud {
	   		method := os.Args[2]
	   		switch method {
	   		case "show":
	   			server, err := lnclient.GetLinode(apikeyring, os.Args[3])
	   			if err != nil {
	   				log.Fatal(err)
	   			}
	   			fmt.Printf("%+v\n", server)
	   			os.Exit(0)
	   		case "list":
	   			all, err := lnclient.ListLinodes(apikeyring)
	   			if err != nil {
	   				log.Fatal(err)
	   			}
	   			for i := range all.Data {
	   				fmt.Printf("ID: %v\n", all.Data[i].Id)
	   			}
	   			os.Exit(0)
	   		case "rm":
	   			err := lnclient.DeleteLinode(apikeyring, os.Args[3])
	   			if err != nil {
	   				log.Fatalf("Couldnt delete linode: %v. Error: %s", os.Args[3], err)
	   			}
	   		}
	   	}
	*/
}
