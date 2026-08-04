package postpass

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
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
func Worker(db *sql.DB, id int, tasks <-chan WorkItem, metrics *Metrics) {
	Idle[id/100].Add(1)
	var query_duration time.Duration

	// reads job from channel
	for task := range tasks {
		taskCtx, cancelTask := context.WithCancel(context.Background())
		go func() {
			for range task.closer {
				cancelTask()
			}
		}()

		startTime := time.Now()
		in_queue := time.Since(task.when_queued)

		metrics.QueueSize.WithLabelValues(task.queue).Dec()

		// log.Printf("worker %d processing task '%s'\n", id, task.request)
		Idle[id/100].Add(-1)

		// this executes the request on the database.
		var rows *sql.Rows
		var err error

		// this makes Postgres create GeoJSON for individual rows,
		// and aggregates them into a collection here instead of
		// using Postgres' json_agg function.
		// A separate query is therefore needed to access the
		// metadata.

		var res = ""
		var builder strings.Builder
		var comma string
		var line string
		var jsonfunc string

		builder.WriteString("{ \n")
		builder.WriteString(`"postpass_properties": { "generator": "Postpass API 0.2"`)

		rows, err = db.QueryContext(taskCtx,
			"SELECT value from osm2pgsql_properties where property='replication_timestamp'")

		if err == nil {
			// If there is an error here, don't include `timestamp` property.
			// this also supports databases which don't have this table.

			// if no rows are returned, no replication_timestamp has been set.
			if rows.Next() {
				err = rows.Scan(&res)
				if err != nil {
					goto sqlerror
				}
			}
			_ = rows.Close()

			builder.WriteString(`, "timestamp": "`)
			builder.WriteString(res)
			builder.WriteString(`"`)
		}

		builder.WriteString(` }, `)
		builder.WriteString("\n")
		if task.geojson {
			builder.WriteString(`"type": "FeatureCollection", "features" : [ `)
			jsonfunc = "ST_AsGeoJSON"
		} else {
			builder.WriteString(`"result" : [ `)
			jsonfunc = "row_to_json"
		}
		builder.WriteString("\n")

		rows, err = db.QueryContext(taskCtx, fmt.Sprintf(
			`SELECT %s(t.*) FROM (%s) as t;`, jsonfunc, task.request))
		if err != nil {
			goto sqlerror
		}

		for rows.Next() {
			err = rows.Scan(&line)
			if err != nil {
				break
			}
			builder.WriteString(comma)
			builder.WriteString(line)
			comma = ",\n"
		}

		if err != nil {
			goto sqlerror
		}

		builder.WriteString("\n]}")
		res = builder.String()

		// discard result
		_ = rows.Close()

		// log.Printf("worker %d done\n", id)

		// send response back on channel
		metrics.RespSent.WithLabelValues(task.queue).Inc()
		query_duration = time.Since(startTime)
		task.response <- SqlResponse{err: false, result: res,
			in_queue: in_queue, query_duration: query_duration, est_cost: task.est_cost, queue: task.queue}
		Idle[id/100].Add(1)
		continue

	sqlerror:
		query_duration = time.Since(startTime)
		metrics.RespSent.WithLabelValues(task.queue).Inc()
		task.response <- SqlResponse{err: true, result: err.Error(),
			in_queue: in_queue, query_duration: query_duration, est_cost: task.est_cost, queue: task.queue}
		Idle[id/100].Add(1)
		continue
	}
}
