package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	configserver "git.aetherial.dev/aeth/yosai/pkg/config-server"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Couldnt load the env file: ", err.Error())
	}

	dbhost := os.Getenv("DB_HOST")
	dbuser := os.Getenv("DB_USER")
	dbpassword := os.Getenv("DB_PASS")
	dbname := "postgres"

	connectionString := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbuser, dbpassword, dbhost, dbname)
	fmt.Print(connectionString)
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	fmt.Println("Successfully connected!")

	configServerDb := configserver.NewSQLiteRepo(db, os.Stdout)

	configServerDb.Migrate()
	conf := config.NewConfiguration(os.Stdout, config.ValidateUsername("aeth"))
	config.NewConfigHostImpl("./.config.json").Propogate(conf)
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
	configserver.RunHttpServer(8080, configServerDb, os.Stdout)

}
