package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	"git.aetherial.dev/aeth/yosai/pkg/daemon"
)

func main() {
	dbfile := "sqlite.db"
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		log.Fatal(err)
	}
	configServerDb := config.NewSQLiteRepo(db, os.Stdout)
	conf := daemon.ReadConfig("./.config.json")
	configServerDb.Migrate()
	user, err := configServerDb.AddUser("aeth")
	if err != nil {
		log.Fatal(err)
	}
	err = configServerDb.SeedUser(user, *conf)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Database created and seeded.")
	dbConf, err := configServerDb.GetConfigByUser("aeth")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", dbConf)

}
