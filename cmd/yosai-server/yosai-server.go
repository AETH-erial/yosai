package main

import (
	"database/sql"
	"log"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/config"
)

func main() {
	dbfile := "sqlite.db"
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		log.Fatal(err)
	}
	configServerDb := config.NewSQLiteRepo(db, os.Stdout)
	configServerDb.Migrate()
	/*
		conf := daemon.NewConfigHostImpl("./.config.json").Get()
		user, err := configServerDb.AddUser(config.ValidateUsername("aeth"))
		if err != nil {
			log.Fatal(err.Error(), "failed to add user")
		}

		configServerDb.SeedUser(user, *conf)
		fmt.Println("Database created and seeded.")
		dbConf, err := configServerDb.GetConfigByUser("aeth")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%+v\n", dbConf)
	*/
	config.RunHttpServer(8080, configServerDb, os.Stdout)

}
