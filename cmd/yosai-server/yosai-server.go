package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	configserver "git.aetherial.dev/aeth/yosai/pkg/config-server"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	envFile := flag.String("env", "", "pass this to read an env file")
	dbSeed := flag.String("seed", "", "Pass this to seed the database with a configuration file")
	flag.Parse()
	if *envFile == "" {
		fmt.Println("No env file passed, attempting to run with raw environment")
	} else {
		err := godotenv.Load(*envFile)
		if err != nil {
			log.Fatal("Couldnt load the env file: ", err.Error())
		}
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
	if *dbSeed != "" {
		conf := config.NewConfiguration(os.Stdout, config.ValidateUsername("aeth"))
		config.NewConfigHostImpl("./.config.json").Propogate(conf)
		user, err := configServerDb.AddUser(config.ValidateUsername("aeth"))
		if err != nil {
			log.Fatal(err.Error(), "failed to add user")
		}

		configServerDb.SeedUser(user, *conf)
		fmt.Println("Database created and seeded.")
	}
	configserver.RunHttpServer(8080, configServerDb, os.Stdout)

}
