package config

import (
	"database/sql"
	"errors"
	"io"

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
	AddUser(Username) error
	UpdateUser(daemon.ConfigFromFile) error
	Log(...string)
	GetConfigByUser() (daemon.ConfigFromFile, error)
}

type SQLiteRepo struct {
	db  *sql.DB
	out io.Writer
}

func (s *SQLiteRepo) Log(msg ...string) {
	logMsg := "SQL Lite log:"
	for i := range msg {
		logMsg = logMsg + msg[i]
	}
	s.out.Write([]byte(logMsg))

}

// Instantiate a new SQLiteRepo struct
func NewSQLiteRepo(db *sql.DB) *SQLiteRepo {
	return &SQLiteRepo{
		db: db,
	}

}

func (s *SQLiteRepo) Migrate() {

	userTable := `
    CREATE TABLE IF NOT EXISTS users(
        id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
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
	    user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		wan_ipv4 TEXT NOT NULL,
		vpn_ipv4 TEXT NOT NULL,
		port INTEGER NOT NULL  
	);`

	clientTable := `

	CREATE TABLE IF NOT EXISTS clients(
	    user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		pubkey TEXT NOT NULL,
		vpn_ipv4 TEXT NOT NULL,
		default BOOLEAN NOT NULL
	);`

	vpnTable := `
    CREATE TABLE IF NOT EXISTS vpn(
	    vpn_ip TEXT NOT NULL,
		vpn_subnet_mask INTEGER NOT NULL
	);
	`
	queries := []string{
		userTable,
		cloudTable,
		ansibleTable,
		serverTable,
		clientTable,
		vpnTable,
	}
	for i := range queries {
		_, err := s.db.Exec(queries[i])
		if err != nil {
			s.Log(err.Error())
		}
	}
}

/*
Add a user to the database and return a User struct

	:param name: the name of the user
*/
func (s *SQLiteRepo) AddUser(name Username) (User, error) {
	var user User
	res, err := s.db.Exec("INSERT INTO users(name) values(?)", name)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if errors.Is(sqliteErr.ExtendedCode, sqlite3.ErrConstraintUnique) {
				return user, ErrDuplicate
			}
		}
		return user, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return user, err
	}
	return User{Name: name, Id: int(id)}, nil
}
