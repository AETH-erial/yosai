package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"git.aetherial.dev/aeth/yosai/pkg/config"
	configserver "git.aetherial.dev/aeth/yosai/pkg/config-server"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	envFile := flag.String("env", "", "pass this to read an env file")
	username := flag.String("username", "", "The username to seed the db with, only has affect when calling with the --seed flag")
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
	dbport := os.Getenv("DB_PORT")
	dbuser := os.Getenv("DB_USER")
	dbpassword := os.Getenv("DB_PASS")
	dbname := "postgres"

	var portInt int
	portInt, err := strconv.Atoi(dbport)
	if err != nil {
		fmt.Println("An unuseable port was passed: '", dbport, "', defaulting to postgres default of 5432")
		portInt = 5432
	}
	if portInt < 0 || portInt > 65535 {
		fmt.Println("An unuseable port was passed: '", dbport, "', defaulting to postgres default of 5432")
		portInt = 5432
	}

	connectionString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", dbuser, dbpassword, dbhost, portInt, dbname)
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
		if *username == "" {
			log.Fatal("Blank username not accepted. Use the --username flag to pass in a username to seed the database with")
		}
		conf := config.NewConfiguration(os.Stdout, config.ValidateUsername(*username))
		config.NewConfigHostImpl("./.config.json").Propogate(conf)
		user, err := configServerDb.AddUser(config.ValidateUsername(*username))
		if err != nil {
			log.Fatal(err.Error(), "failed to add user")
		}

		configServerDb.SeedUser(user, *conf)
		fmt.Println("Database created and seeded.")
	}
	configserver.RunHttpServer(8080, configServerDb, os.Stdout)

}
