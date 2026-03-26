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
	var content_type string
	var err error
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

		if task.output_format == "geojson" {
			res, err = geojson_output(db, taskCtx, task)
			content_type = "application/geojson"
		} else {
			panic(fmt.Sprintf("Unsupported output_format: %s", task.output_format))
		}

		if err != nil {
			task.response <- SqlResponse{err: true, result: err.Error()}
			Idle[id/100].Add(1)
			continue
		}

		// send response back on channel
		task.response <- SqlResponse{err: false, content_type: content_type, result: res}
		Idle[id/100].Add(1)
        continue

	}
}

func geojson_output(db *sql.DB, taskCtx context.Context, task WorkItem) (string, error) {
		// this executes the request on the database.
		var rows *sql.Rows
		var res string
		var err error

		var builder strings.Builder
		var line string
		row_num := 0

		// “header” of GeoJSON output

		builder.WriteString("{ \"type\": \"FeatureCollection\", \"properties\": { \"generator\": \"Postpass API 0.2\", \"timestamp\": \"")

		// output the timestamp
		rows, err = db.QueryContext(taskCtx, "select value from osm2pgsql_properties where property='replication_timestamp'")
		if err != nil {
			return "", err
		}
		rows.Next()
		err = rows.Scan(&res)
		if err != nil {
			return "", err
		}
		_ = rows.Close()
		builder.WriteString(res)

		builder.WriteString("\"}, \"features\": [")

		// Now do each row
				
		rows, err = db.QueryContext(taskCtx, fmt.Sprintf(
			`SELECT ST_AsGeoJSON(t.*) FROM (%s) as t;`, task.request))

		if err != nil {
			return "", err
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
			return "", err
		}


		// “footer” of GeoJSON
		builder.WriteString("]}");

		if err != nil {
			return "", err
		}

		// discard result
		_ = rows.Close()

		res = builder.String()

		
		return res, err
}
