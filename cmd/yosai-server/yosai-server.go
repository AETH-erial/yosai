package main

import (
	"database/sql"
	"fmt"
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
	fmt.Println("Database created and seeded.")
	dbConf, err := configServerDb.GetConfigByUser("aeth")
	if err != nil {
		log.Fatal(err)
	}
	config.RunHttpServer(8080, configServerDb, os.Stdout)
	fmt.Printf("%+v\n", dbConf)

}
