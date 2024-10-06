package config

import (
	"database/sql"

	"git.aetherial.dev/aeth/yosai/pkg/daemon"
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
	Migrate() error
	AddUser(user User) error
	UpdateCloud(daemon.ConfigFromFile) error
	UpdateAnsible(daemon.ConfigFromFile) error
	UpdateService(daemon.ConfigFromFile) error
	Log(...string)
	AddConfigForUser(daemon.ConfigFromFile, error)
	GetConfigByUser() (daemon.ConfigFromFile, error)
}

type SQLiteRepo struct {
	db *sql.DB
}

// Instantiate a new SQLiteRepo struct
func NewSQLiteRepo(db *sql.DB) *SQLiteRepo {
	return &SQLiteRepo{
		db: db,
	}

}

func (s *SQLiteRepo) AddUser(user User) error {

	singleUser := `
    CREATE TABLE IF NOT EXISTS ?(
        id INTEGER NOT NULL,
		name TEXT NOT NULL,
        cloud_config_id INTEGER NOT NULL,
        ansible_config_id INTEGER NOT NULL UNIQUE,
        servers_id INTEGER NOT NULL,
		clients_id INTEGER NOT NULL,
		vpn_config_id INTEGER NOT NULL,
		network TEXT NOT NULL
    );
    `

	cloudTable := `
	CREATE TABLE IF NOT EXISTS cloud(
	    user_id INTEGER NOT NULL,
		image TEXT NOT NULL,
		region TEXT NOT NULL,
		linode_type TEXT NOT NULL  
	);`

	ansibleTable := `
	CREATE TABLE IF NOT EXISTS ansible(
	    user_id INTEGER NOT NULL,
		repo_url TEXT NOT NULL,
		branch TEXT NOT NULL,
		playbook_name TEXT NOT NULL,
		ansible_backend TEXT NOT NULL,
		ansible_backend_url TEXT NOT NULL
	);`
	serverTable := `
	CREATE TABLE IF NOT EXISTS servers(
	    
	)`
	clientTable := ``
	vpnTable := ``
	// Update the users ID and propogate it into the id for each of the reference tables
	_, err := s.db.Exec(singleUser, user.Name)
	return err

}

// Creates a new SQL table with necessary data
func (s *SQLiteRepo) Init() error {
	userTable := `
    CREATE TABLE IF NOT EXISTS users(
	    name TEXT PRIMARY KEY,
		id INTEGER AUTOINCREMENT
	);
	`

	_, err := s.db.Exec(userTable)
	return err
}
