/*
 * "postpass"
 *
 * a simple wrapper around PostGIS that allows random people on the
 * internet to run PostGIS queries without ruining everything
 *
 * written by Frederik Ramm, GPL3+
 */

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/goccy/go-yaml"
	_ "github.com/lib/pq"
	"postpass/postpass"
)

/*
 * main program
 */
func main() {

	// don't log timestamp since systemd already does
	log.SetFlags(0)

	var configPath = flag.String("c", "", "Filepath of the config file")
	var printConfig = flag.Bool("print-config", false, "Print the out the config")
	var dbHost = flag.String("db-host", "", "The postgis database host")
	var dbPort = flag.Int("db-port", 0, "The port of the postgis database")
	var dbUser = flag.String("db-username", "", "The username of the postgis user")
	var dbPassword = flag.String("db-password", "", "The password of the postgis user")
	var dbName = flag.String("db-name", "", "The name of the postgis database")
	flag.Parse()

	// Load config from file
	var cfgPath *string
	if *configPath != "" {
		cfgPath = configPath
	}
	cfg, err := postpass.LoadConfig(cfgPath)
	if err != nil {
		log.Printf("error loading config file: %s\n", err)
		panic(err)
	}

	// Override config file values if set from cli
	if *dbHost != "" {
		cfg.Database.Host = *dbHost
	}
	if *dbPort != 0 {
		cfg.Database.Port = *dbPort
	}
	if *dbUser != "" {
		cfg.Database.User = *dbUser
	}
	if *dbPassword != "" {
		cfg.Database.Password = *dbPassword
	}
	if *dbName != "" {
		cfg.Database.DatabaseName = *dbName
	}

	if *printConfig {
		out, err := yaml.Marshal(cfg)
		if err != nil {
			log.Printf("error serializing config: %s\n", err)
			panic(err)
		}
		fmt.Print(string(out))
		return
	}

	// open a connection to the database
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable options='-c statement_timeout=36000000'",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User, cfg.Database.Password, cfg.Database.DatabaseName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("error closing db: %s\n", err.Error())
		}
	}()

	db.SetMaxIdleConns(100)
	db.SetMaxOpenConns(200)
	db.SetConnMaxLifetime(time.Hour)

	// verify the connection
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	// initialize goroutines
	quick_jobs := make(chan postpass.WorkItem, 50)
	for w := 1; w <= 10; w++ {
		go postpass.Worker(db, 100+w, quick_jobs)
	}
	medium_jobs := make(chan postpass.WorkItem, 50)
	for w := 1; w <= 4; w++ {
		go postpass.Worker(db, 200+w, medium_jobs)
	}
	slow_jobs := make(chan postpass.WorkItem, 50)
	for w := 1; w <= 2; w++ {
		go postpass.Worker(db, 300+w, slow_jobs)
	}

	// set up a ticker to log how many busy workers there are
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			<-ticker.C
			log.Printf("idle workers: %d/10 quick, %d/4 medium, %d/2 slow; request count: %d\n",
				postpass.Idle[1].Load(), postpass.Idle[2].Load(), postpass.Idle[3].Load(), postpass.Count.Load())
		}
	}()

	// set up callback for /interpreter URL
	http.HandleFunc("/interpreter", func(w http.ResponseWriter, r *http.Request) {
		postpass.HandleInterpreter(db, slow_jobs, medium_jobs, quick_jobs, &cfg, w, r)
	})
	// set up callback for /explain URL
	http.HandleFunc("/explain", func(w http.ResponseWriter, r *http.Request) {
		postpass.HandleExplain(db, &cfg, w, r)
	})

	http.HandleFunc("/healthy", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "healthy\n")
	})

	log.Printf("Listening on :%d", cfg.ListenPort)
	// endless loop
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.ListenPort), nil))
}
