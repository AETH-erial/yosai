package configserver

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"git.aetherial.dev/aeth/yosai/pkg/config"
)

var (
	ErrDuplicate    = errors.New("record already exists")
	ErrNotExists    = errors.New("row not exists")
	ErrUpdateFailed = errors.New("update failed")
	ErrDeleteFailed = errors.New("delete failed")
)

type DatabaseIO interface {
	Migrate()
	AddUser(config.Username) (config.User, error)
	UpdateUser(config.Username, config.Configuration) error
	Log(...string)
	GetConfigByUser(config.Username) (config.Configuration, error)
}

type SQLiteRepo struct {
	db  *sql.DB
	out io.Writer
}

/*
Create a new SQL lite repo

	:param db: a pointer to a sql.DB to write the database into
*/
func NewSQLiteRepo(db *sql.DB, out io.Writer) *SQLiteRepo {
	return &SQLiteRepo{
		db:  db,
		out: out,
	}

}

func (s *SQLiteRepo) Log(msg ...string) {
	data := "config.SQLiteRepo: "
	for i := range msg {
		data = data + " " + msg[i] + "\n"
	}
	s.out.Write([]byte(fmt.Sprintf(LogMsgTmpl, time.Now().String(), data)))

}

func (s *SQLiteRepo) Migrate() {

	userTable := `
    CREATE TABLE IF NOT EXISTS users(
        id SERIAL PRIMARY KEY,
		name TEXT NOT NULL
    );
    `

	cloudTable := `
	CREATE TABLE IF NOT EXISTS cloud(
	    user_id INTEGER NOT NULL,
		image TEXT NOT NULL,
		region TEXT NOT NULL,
		linode_type TEXT NOT NULL  
	);
	`

	ansibleTable := `
	CREATE TABLE IF NOT EXISTS ansible(
	    user_id INTEGER NOT NULL,
		repo_url TEXT NOT NULL,
		branch TEXT NOT NULL,
		playbook_name TEXT NOT NULL,
		ansible_backend TEXT NOT NULL,
		ansible_backend_url TEXT NOT NULL
	);
	`
	serverTable := `
	CREATE TABLE IF NOT EXISTS servers(
	    user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		wan_ipv4 TEXT NOT NULL,
		vpn_ipv4 TEXT NOT NULL
	);
	`

	clientTable := `
	CREATE TABLE IF NOT EXISTS clients(
	    user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		pubkey TEXT NOT NULL,
		vpn_ipv4 TEXT NOT NULL,
		default_client BOOLEAN NOT NULL
	);
	`

	serviceTable := `
    CREATE TABLE IF NOT EXISTS service(
	    user_id INTEGER NOT NULL,
	    vpn_ip TEXT NOT NULL,
		vpn_subnet_mask INTEGER NOT NULL,
		vpn_server_port INTEGER NOT NULL,
		secrets_backend TEXT NOT NULL,
		secrets_backend_url TEXT NOT NULL
	);
	`
	queries := []string{
		userTable,
		cloudTable,
		ansibleTable,
		serverTable,
		clientTable,
		serviceTable,
	}
	for i := range queries {
		_, err := s.db.Exec(queries[i])
		if err != nil {
			s.Log(err.Error())
		}
	}
}

/*
Retrieve a user struct from, querying by their username

	:param name: the username of the querying user Note -> must validate the username before calling
*/
func (s *SQLiteRepo) GetUser(name config.Username) (config.User, error) {
	row := s.db.QueryRow("SELECT * FROM users WHERE name = $1", name)

	var user config.User
	if err := row.Scan(&user.Id, &user.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, ErrNotExists
		}
		return user, err
	}
	return user, nil
}

/*
Update all of the data for a users configuration

		:param config: a config.Configuration to put into the database
	    :param user: the config.User struct representing the calling user
*/
func (s *SQLiteRepo) UpdateUser(username config.Username, config config.Configuration) error {

	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Error creating DB transaction: ", err.Error())
		return err
	}
	defer trx.Rollback()
	user, err := s.GetUser(username)
	if err != nil {
		s.Log("Error getting the user: ", string(username), err.Error())
		return err
	}
	_, err = trx.Exec("UPDATE cloud SET image = $1, region = $2, linode_type = $3 WHERE user_id = $4",
		config.Cloud.Image,
		config.Cloud.Region,
		config.Cloud.LinodeType,
		user.Id)
	if err != nil {
		return err
	}
	_, err = trx.Exec("UPDATE ansible SET repo_url = $1, branch = $2, playbook_name = $3, ansible_backend = $4, ansible_backend_url = $5 WHERE user_id = $6",
		config.Ansible.Repo,
		config.Ansible.Branch,
		config.Ansible.PlaybookName,
		config.Service.AnsibleBackend,
		config.Service.AnsibleBackendUrl,
		user.Id)
	if err != nil {
		return err
	}
	_, err = trx.Exec("DELETE FROM servers WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to drop the users server entries: ", err.Error())
		return err
	}
	err = s.insertServer(user, config, trx)
	if err != nil {
		s.Log("Failed to propogate the VPN servers into the appropriate table: ", err.Error())
		return err
	}
	_, err = trx.Exec("DELETE FROM clients WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to drop the users client entries: ", err.Error())
		return err
	}
	err = s.insertClient(user, config, trx)
	if err != nil {
		s.Log("Failed to propogate the VPN clients into the appropriate table: ", err.Error())
		return err
	}

	_, err = trx.Exec("UPDATE service SET vpn_ip = $1, vpn_subnet_mask = $2, vpn_server_port = $3, secrets_backend = $4, secrets_backend_url = $5 WHERE user_id = $6",
		config.Service.VpnAddressSpace.String(),
		config.Service.VpnMask,
		config.Service.VpnServerPort,
		config.Service.SecretsBackend,
		config.Service.SecretsBackendUrl,
		user.Id)
	if err != nil {
		return err
	}
	err = trx.Commit()
	if err != nil {
		return err
	}
	s.Log("Transaction commited.")

	return nil
}

/*
Create an entry in the vpn information table

	    :param user: the calling config.User
		:param config: the config.Configuration with the configuration data
*/
func (s *SQLiteRepo) insertServiceInfo(user config.User, config config.Configuration, trx *sql.Tx) error {
	rows, err := trx.Query("SELECT * FROM service WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}

	_, err = trx.Exec("INSERT INTO service(user_id, vpn_ip, vpn_subnet_mask, vpn_server_port, secrets_backend, secrets_backend_url) values($1,$2,$3,$4,$5,$6)",
		user.Id,
		config.Service.VpnAddressSpace.String(),
		config.Service.VpnMask,
		config.Service.VpnServerPort,
		config.Service.SecretsBackend,
		config.Service.SecretsBackendUrl)
	if err != nil {
		s.Log(string(user.Id), config.Service.VpnAddressSpace.String(), string(config.Service.VpnMask), string(config.Service.VpnServerPort), config.Service.SecretsBackend, config.Service.SecretsBackendUrl)
		s.Log("Error occured writing to the database: ", err.Error())
		return err
	}
	return nil
}

/*
Create an entry in the client table for a user

	    :param user: the calling config.User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertClient(user config.User, config config.Configuration, trx *sql.Tx) error {
	rows, err := trx.Query("SELECT * FROM clients WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}
	for i := range config.Service.Clients {
		client := config.Service.Clients[i]
		_, err = trx.Exec("INSERT INTO clients(user_id, name, pubkey, vpn_ipv4, default_client) values($1,$2,$3,$4,$5)",
			user.Id,
			client.Name,
			client.Pubkey,
			client.VpnIpv4.String(),
			client.Default)
		if err != nil {
			s.Log(fmt.Sprintf("%+v\n", client), fmt.Sprintf("%+v\n", user))
			s.Log("Failed to create row: ", err.Error())
			return err
		}
	}
	return nil

}

/*
Create an entry in the server table for a user

	    :param user: the calling config.User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertServer(user config.User, config config.Configuration, trx *sql.Tx) error {
	rows, err := trx.Query("SELECT * FROM servers WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}
	for i := range config.Service.Servers {
		server := config.Service.Servers[i]
		_, err = trx.Exec("INSERT INTO servers(user_id, name, wan_ipv4, vpn_ipv4) values($1,$2,$3,$4)",
			user.Id,
			server.Name,
			server.WanIpv4,
			server.VpnIpv4.String())
		if err != nil {
			s.Log("Failed to create row: ", err.Error())
			return err
		}
	}
	return nil

}

/*
Create an entry in the ansible table for a user

	    :param user: the calling config.User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertUserAnsible(user config.User, config config.Configuration, trx *sql.Tx) error {
	rows, err := trx.Query("SELECT * FROM ansible WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}
	_, err = trx.Exec("INSERT INTO ansible(user_id, repo_url, branch, playbook_name, ansible_backend, ansible_backend_url) values($1,$2,$3,$4,$5,$6)",
		user.Id,
		config.Ansible.Repo,
		config.Ansible.Branch,
		config.Ansible.PlaybookName,
		config.Service.AnsibleBackend,
		config.Service.AnsibleBackendUrl)
	if err != nil {
		s.Log("Failed to create row: ", err.Error())
		return err
	}
	return nil

}

/*
Create an entry in the cloud table for a user

	    :param user: the calling config.User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertUserCloud(user config.User, config config.Configuration, trx *sql.Tx) error {
	rows, err := trx.Query("SELECT * FROM cloud WHERE user_id = $1", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}
	_, err = trx.Exec("INSERT INTO cloud(user_id, image, region, linode_type) values($1,$2,$3,$4)",
		user.Id,
		config.Cloud.Image,
		config.Cloud.Region,
		config.Cloud.LinodeType)
	if err != nil {
		s.Log("Failed to create row: ", err.Error())
		return err
	}
	return nil

}

/*
Populate the different db tables with the users configuration

	    :param user: the calling user
		:param config: the config.Configuration to populate into the db
*/
func (s *SQLiteRepo) SeedUser(user config.User, cfg config.Configuration) error {
	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Failed to spawn a transaction in SQLiteRepo.SeedUser: ", err.Error())
		return err
	}
	seedFuncs := []func(config.User, config.Configuration, *sql.Tx) error{
		s.insertClient,
		s.insertServer,
		s.insertUserAnsible,
		s.insertUserCloud,
		s.insertServiceInfo,
	}
	for i := range seedFuncs {
		err := seedFuncs[i](user, cfg, trx)
		if err != nil {
			return err
		}
	}
	err = trx.Commit()
	if err != nil {
		return err
	}
	s.Log("Transaction commited.")
	return nil

}

/*
Add a user to the database and return a config.User struct

	:param name: the name of the user
*/
func (s *SQLiteRepo) AddUser(name config.Username) (config.User, error) {
	var user config.User
	_, err := s.db.Exec("INSERT INTO users(name) values($1)", name)
	if err != nil {
		return user, err
	}
	return s.GetUser(name)
}

/*
Get the configuration for the passed user

	:param user: the calling user
*/
func (s *SQLiteRepo) GetConfigByUser(username config.Username) (config.Configuration, error) {
	cfg := config.NewConfiguration(bytes.NewBuffer([]byte{}), username)
	user, err := s.GetUser(username)
	if err != nil {
		return *cfg, err
	}
	row := s.db.QueryRow("SELECT * FROM cloud WHERE user_id = $1", user.Id)
	if err := row.Scan(&user.Id, &cfg.Cloud.Image, &cfg.Cloud.Region, &cfg.Cloud.LinodeType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return *cfg, ErrNotExists
		}
		return *cfg, err
	}
	row = s.db.QueryRow("SELECT * FROM ansible WHERE user_id = $1", user.Id)
	if err := row.Scan(
		&user.Id,
		&cfg.Ansible.Repo,
		&cfg.Ansible.Branch,
		&cfg.Ansible.PlaybookName,
		&cfg.Service.AnsibleBackend,
		&cfg.Service.AnsibleBackendUrl); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return *cfg, ErrNotExists
		}
		return *cfg, err
	}
	rows, err := s.db.Query("SELECT * FROM servers WHERE user_id = $1", user.Id)
	if err != nil {
		return *cfg, err
	}
	for rows.Next() {
		var server config.VpnServer
		var ipStr string
		if err := rows.Scan(&user.Id, &server.Name, &server.WanIpv4, &ipStr); err != nil {
			return *cfg, err
		}
		s.Log(ipStr)
		ipParsed := net.ParseIP(ipStr)
		if ipParsed == nil {
			s.Log("Couldnt parse address: ", ipStr, err.Error())
		}
		server.VpnIpv4 = ipParsed
		cfg.Service.Servers[server.Name] = server
	}
	if err = rows.Err(); err != nil {
		return *cfg, err
	}
	rows, err = s.db.Query("SELECT * FROM clients WHERE user_id = $1", user.Id)
	if err != nil {
		return *cfg, err
	}
	for rows.Next() {
		var client config.VpnClient
		var ipStr string
		if err := rows.Scan(&user.Id, &client.Name, &client.Pubkey, &ipStr, &client.Default); err != nil {
			return *cfg, err
		}
		s.Log(ipStr)
		ipParsed := net.ParseIP(ipStr)
		if ipParsed == nil {
			s.Log("Couldnt parse address: ", ipStr, err.Error())
		}
		client.VpnIpv4 = ipParsed
		cfg.Service.Clients[client.Name] = client
	}
	row = s.db.QueryRow("SELECT * FROM service WHERE user_id = $1", user.Id)
	var vpnIp string
	if err = row.Scan(&user.Id, &vpnIp, &cfg.Service.VpnMask, &cfg.Service.VpnServerPort, &cfg.Service.SecretsBackend, &cfg.Service.SecretsBackendUrl); err != nil {
		return *cfg, err
	}
	_, vpnIpv4, _ := net.ParseCIDR(vpnIp)
	cfg.Service.VpnAddressSpace = *vpnIpv4

	return *cfg, nil

}
