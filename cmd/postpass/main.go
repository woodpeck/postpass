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
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"postpass/postpass"
)

/*
 * main program
 */
func main() {

	// don't log timestamp since systemd already does
	log.SetFlags(0)

	// open a connection to the database
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable options='-c statement_timeout=36000000'",
		postpass.Host, postpass.Port, postpass.User, postpass.Password, postpass.DBName)
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
		postpass.HandleInterpreter(db, slow_jobs, medium_jobs, quick_jobs, w, r)
	})
	// set up callback for /explain URL
	http.HandleFunc("/explain", func(w http.ResponseWriter, r *http.Request) {
		postpass.HandleExplain(db, w, r)
	})

	log.Printf("Listening on :%d", postpass.ListenPort)
	// endless loop
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", postpass.ListenPort), nil))
}
