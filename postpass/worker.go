package postpass

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
)

// global request counter
var Count atomic.Int64

// global counter for idle workers
var Idle [4]atomic.Int64

/*
 * worker function that executes SQL queries
 *
 * arguments: database connection, worker id, channel to read jobs from
 */
func Worker(db *sql.DB, id int, tasks <-chan WorkItem) {
	var res string
	Idle[id/100].Add(1)

	// reads job from channel
	for task := range tasks {
		taskCtx, cancelTask := context.WithCancel(context.Background())
		go func() {
			for range task.closer {
				cancelTask()
			}
		}()

		// log.Printf("worker %d processing task '%s'\n", id, task.request)
		Idle[id/100].Add(-1)

		// this executes the request on the database.
		var rows *sql.Rows
		var err error


		var builder strings.Builder
		var line string
		row_num := 0

		// “header” of GeoJSON output

		builder.WriteString("{ \"type\": \"FeatureCollection\", \"properties\": { \"generator\": \"Postpass API 0.2\", \"timestamp\": \"")

		// output the timestamp
		rows, err = db.QueryContext(taskCtx, "select value from osm2pgsql_properties where property='replication_timestamp'")
		if err != nil {
			goto sqlerror
		}
		rows.Next()
		err = rows.Scan(&res)
		if err != nil {
			goto sqlerror
		}
		_ = rows.Close()
		builder.WriteString(res)

		builder.WriteString("\"}, \"features\": [")

		// Now do each row
				
		rows, err = db.QueryContext(taskCtx, fmt.Sprintf(
			`SELECT ST_AsGeoJSON(t.*) FROM (%s) as t;`, task.request))

		if err != nil {
			goto sqlerror
		}

		for rows.Next() {
			err = rows.Scan(&line)
			if err != nil {
				break;
			}

			if row_num >= 1 {
				builder.WriteString(", ")
			}

			builder.WriteString(line);
			row_num ++
		}

		if err != nil {
			goto sqlerror
		}


		// “footer” of GeoJSON
		builder.WriteString("]}");

		if err != nil {
			goto sqlerror
		}

		// discard result
		_ = rows.Close()

		res = builder.String()

		// log.Printf("worker %d done\n", id)

		// send response back on channel
		task.response <- SqlResponse{err: false, result: res}
		Idle[id/100].Add(1)
        continue

        sqlerror:
        task.response <- SqlResponse{err: true, result: err.Error()}
        Idle[id/100].Add(1)
        continue
	}
}
