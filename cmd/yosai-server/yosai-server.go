package main

import (
	"database/sql"
	"log"
	"os"

	configserver "git.aetherial.dev/aeth/yosai/pkg/config-server"
)

func main() {
	dbfile := "sqlite.db"
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		log.Fatal(err)
	}
	configServerDb := configserver.NewSQLiteRepo(db, os.Stdout)
	configServerDb.Migrate()
	/*
		conf := daemon.NewConfiguration(os.Stdout)
		daemon.NewConfigHostImpl("./.config.json").Propogate(conf)
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
	configserver.RunHttpServer(8080, configServerDb, os.Stdout)

}
