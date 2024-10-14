package config

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
	"github.com/mattn/go-sqlite3"
)

var (
	ErrDuplicate    = errors.New("record already exists")
	ErrNotExists    = errors.New("row not exists")
	ErrUpdateFailed = errors.New("update failed")
	ErrDeleteFailed = errors.New("delete failed")
)

type Username string

func ValidateUsername(name string) Username {
	return Username(name)
}

type User struct {
	Name Username
	Id   int
}

type DatabaseIO interface {
	Migrate()
	AddUser(Username) (User, error)
	UpdateUser(Username, daemon.Configuration) error
	Log(...string)
	GetConfigByUser(Username) (daemon.Configuration, error)
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
	logMsg := "SQL Lite log:"
	for i := range msg {
		logMsg = logMsg + msg[i]
	}
	logMsg = logMsg + "\n"
	s.out.Write([]byte(logMsg))

}

func (s *SQLiteRepo) Migrate() {

	userTable := `
    CREATE TABLE IF NOT EXISTS users(
        id INTEGER PRIMARY KEY AUTOINCREMENT,
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
		vpn_ipv4 TEXT NOT NULL,
		port INTEGER NOT NULL  
	);
	`

	clientTable := `
	CREATE TABLE IF NOT EXISTS clients(
	    user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		pubkey TEXT NOT NULL,
		vpn_ipv4 TEXT NOT NULL,
		default_client INTEGER NOT NULL
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
func (s *SQLiteRepo) GetUser(name Username) (User, error) {
	row := s.db.QueryRow("SELECT * FROM users WHERE name = ?", name)

	var user User
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

		:param config: a daemon.Configuration to put into the database
	    :param user: the User struct representing the calling user
*/
func (s *SQLiteRepo) UpdateUser(username Username, config daemon.Configuration) error {

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
	_, err = trx.Exec("UPDATE cloud SET user_id = ?, image = ?, region = ?, linode_type = ? WHERE user_id = ?",
		user.Id,
		config.Cloud.Image,
		config.Cloud.Region,
		config.Cloud.LinodeType,
		user.Id)
	if err != nil {
		return err
	}
	_, err = trx.Exec("UPDATE ansible SET user_id = ?, repo_url = ?, branch = ?, playbook_name = ?, ansible_backend = ?, ansible_backend_url = ? WHERE user_id = ?",
		user.Id,
		config.Ansible.Repo,
		config.Ansible.Branch,
		config.Ansible.PlaybookName,
		config.Service.AnsibleBackend,
		config.Service.AnsibleBackendUrl,
		user.Id)
	if err != nil {
		return err
	}
	for i := range config.Service.Servers {
		server := config.Service.Servers[i]
		_, err := trx.Exec("UPDATE servers SET user_id = ?, name = ?, wan_ipv4 = ?, vpn_ipv4 = ?, port = ? WHERE user_id = ? AND name = ?",
			user.Id,
			server.Name,
			server.WanIpv4,
			server.VpnIpv4,
			server.Port,
			user.Id,
			server.Name)
		if err != nil {
			return err
		}
	}
	for i := range config.Service.Clients {
		client := config.Service.Clients[i]
		_, err := trx.Exec("UPDATE clients SET user_id = ?, name = ?, pubkey = ?, vpn_ipv4 = ?, default_client = ? WHERE user_id = ? AND name = ?",
			user.Id,
			client.Name,
			client.Pubkey,
			client.VpnIpv4,
			client.Default,
			user.Id,
			client.Name)
		if err != nil {
			return err
		}
	}
	_, err = trx.Exec("UPDATE service SET user_id = ? , vpn_ip = ?, vpn_subnet_mask = ?, vpn_server_port = ?, secrets_backend = ?, secrets_backend_url = ? WHERE user_id = ?",
		user.Id,
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

	    :param user: the calling User
		:param config: the daemon.Configuration with the configuration data
*/
func (s *SQLiteRepo) insertServiceInfo(user User, config daemon.Configuration) error {
	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Failed to start DB transaction: ", err.Error())
		return err
	}
	defer trx.Rollback()
	rows, err := trx.Query("SELECT * FROM service WHERE user_id = ?", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}

	_, err = trx.Exec("INSERT INTO service(user_id, vpn_ip, vpn_subnet_mask, vpn_server_port, secrets_backend, secrets_backend_url) values(?,?,?,?,?,?)",
		user.Id,
		config.Service.VpnAddressSpace.String(),
		config.Service.VpnMask,
		config.Service.VpnServerPort,
		config.Service.SecretsBackend,
		config.Service.SecretsBackendUrl)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if errors.Is(sqliteErr.ExtendedCode, sqlite3.ErrConstraintUnique) {
				return ErrDuplicate
			}
		}
		return err
	}
	err = trx.Commit()
	if err != nil {
		return err
	}
	return nil
}

/*
Create an entry in the client table for a user

	    :param user: the calling User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertClient(user User, config daemon.Configuration) error {
	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Failed to start DB transaction: ", err.Error())
		return err
	}
	defer trx.Rollback()
	rows, err := trx.Query("SELECT * FROM clients WHERE user_id = ?", user.Id)
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
		_, err = trx.Exec("INSERT INTO clients(user_id, name, pubkey, vpn_ipv4, default_client) values(?,?,?,?,?)",
			user.Id,
			client.Name,
			client.Pubkey,
			client.VpnIpv4,
			client.Default)
		if err != nil {
			s.Log("Failed to create row: ", err.Error())
			return err
		}
	}
	err = trx.Commit()
	if err != nil {
		return err
	}
	return nil

}

/*
Create an entry in the server table for a user

	    :param user: the calling User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertServer(user User, config daemon.Configuration) error {
	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Failed to start DB transaction: ", err.Error())
		return err
	}
	defer trx.Rollback()
	rows, err := trx.Query("SELECT * FROM servers WHERE user_id = ?", user.Id)
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
		_, err = trx.Exec("INSERT INTO servers(user_id, name, wan_ipv4, vpn_ipv4, port) values(?,?,?,?,?)",
			user.Id,
			server.Name,
			server.WanIpv4,
			server.VpnIpv4,
			server.Port)
		if err != nil {
			s.Log("Failed to create row: ", err.Error())
			return err
		}
	}
	err = trx.Commit()
	if err != nil {
		return err
	}
	return nil

}

/*
Create an entry in the ansible table for a user

	    :param user: the calling User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertUserAnsible(user User, config daemon.Configuration) error {
	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Failed to start DB transaction: ", err.Error())
		return err
	}
	defer trx.Rollback()
	rows, err := trx.Query("SELECT * FROM ansible WHERE user_id = ?", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}
	_, err = trx.Exec("INSERT INTO ansible(user_id, repo_url, branch, playbook_name, ansible_backend, ansible_backend_url) values(?,?,?,?,?,?)",
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
	err = trx.Commit()
	if err != nil {
		return err
	}
	return nil

}

/*
Create an entry in the cloud table for a user

	    :param user: the calling User
		:param cloudConfig: the cloud specific configuration for the user
*/
func (s *SQLiteRepo) insertUserCloud(user User, config daemon.Configuration) error {
	trx, err := s.db.Begin()
	if err != nil {
		s.Log("Failed to start DB transaction: ", err.Error())
		return err
	}
	defer trx.Rollback()
	rows, err := trx.Query("SELECT * FROM cloud WHERE user_id = ?", user.Id)
	if err != nil {
		s.Log("Failed to perform pre-insert check", err.Error())
		return err
	}
	if rows.Next() { // Checking if the 'length' of returned rows is non-zero
		s.Log("Duplicate INSERT attempted, update instead.", err.Error())
		return ErrDuplicate
	}
	_, err = trx.Exec("INSERT INTO cloud(user_id, image, region, linode_type) values(?,?,?,?)",
		user.Id,
		config.Cloud.Image,
		config.Cloud.Region,
		config.Cloud.LinodeType)
	if err != nil {
		s.Log("Failed to create row: ", err.Error())
		return err
	}
	err = trx.Commit()
	if err != nil {
		return err
	}
	return nil

}

/*
Populate the different db tables with the users configuration

	    :param user: the calling user
		:param config: the daemon.Configuration to populate into the db
*/
func (s *SQLiteRepo) SeedUser(user User, config daemon.Configuration) error {
	seedFuncs := []func(User, daemon.Configuration) error{
		s.insertClient,
		s.insertServer,
		s.insertUserAnsible,
		s.insertUserCloud,
		s.insertServiceInfo,
	}
	for i := range seedFuncs {
		err := seedFuncs[i](user, config)
		if err != nil {
			return err
		}
	}
	return nil

}

/*
Add a user to the database and return a User struct

	:param name: the name of the user
*/
func (s *SQLiteRepo) AddUser(name Username) (User, error) {
	var user User
	rows, err := s.db.Query("SELECT * FROM users WHERE name = ?", name)
	if err != nil {
		s.Log(err.Error())
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if errors.Is(sqliteErr.ExtendedCode, sqlite3.ErrConstraintUnique) {
				return user, ErrDuplicate
			}
		}
		return user, err
	}
	if rows.Next() {
		s.Log("Duplicate username, please use another")
		return user, ErrDuplicate
	}
	res, err := s.db.Exec("INSERT INTO users(name) values(?)", name)
	if err != nil {
		return user, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return user, err
	}
	return User{Name: name, Id: int(id)}, nil
}

/*
Get the configuration for the passed user

	:param user: the calling user
*/
func (s *SQLiteRepo) GetConfigByUser(username Username) (daemon.Configuration, error) {
	config := daemon.NewConfiguration(bytes.NewBuffer([]byte{}))
	user, err := s.GetUser(username)
	if err != nil {
		return *config, err
	}
	row := s.db.QueryRow("SELECT * FROM cloud WHERE user_id = ?", user.Id)
	if err := row.Scan(&user.Id, &config.Cloud.Image, &config.Cloud.Region, &config.Cloud.LinodeType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return *config, ErrNotExists
		}
		return *config, err
	}
	row = s.db.QueryRow("SELECT * FROM ansible WHERE user_id = ?", user.Id)
	if err := row.Scan(
		&user.Id,
		&config.Ansible.Repo,
		&config.Ansible.Branch,
		&config.Ansible.PlaybookName,
		&config.Service.AnsibleBackend,
		&config.Service.AnsibleBackendUrl); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return *config, ErrNotExists
		}
		return *config, err
	}
	rows, err := s.db.Query("SELECT * FROM servers WHERE user_id = ?", user.Id)
	if err != nil {
		return *config, err
	}
	for rows.Next() {
		var server daemon.VpnServer
		if err := rows.Scan(&user.Id, &server.Name, &server.WanIpv4, &server.VpnIpv4, &server.Port); err != nil {
			return *config, err
		}
		config.Service.Servers[server.Name] = server
	}
	if err = rows.Err(); err != nil {
		return *config, err
	}
	rows, err = s.db.Query("SELECT * FROM clients WHERE user_id = ?", user.Id)
	if err != nil {
		return *config, err
	}
	for rows.Next() {
		var client daemon.VpnClient
		if err := rows.Scan(&user.Id, &client.Name, &client.Pubkey, &client.VpnIpv4, &client.Default); err != nil {
			return *config, err
		}
		config.Service.Clients[client.Name] = client
	}
	row = s.db.QueryRow("SELECT * FROM service WHERE user_id = ?", user.Id)
	var vpnIp string
	if err = row.Scan(&user.Id, &vpnIp, &config.Service.VpnMask, &config.Service.VpnServerPort, &config.Service.SecretsBackend, &config.Service.SecretsBackendUrl); err != nil {
		return *config, err
	}
	_, vpnIpv4, _ := net.ParseCIDR(vpnIp)
	config.Service.VpnAddressSpace = *vpnIpv4

	return *config, nil

}
