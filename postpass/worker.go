package postpass

import (
	"context"
	"database/sql"
	"fmt"
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

		tx, err := db.BeginTx(taskCtx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			task.response <- SqlResponse{err: true, result: err.Error()}
			Idle[id/100].Add(1)
			continue
		}
		defer tx.Rollback()

		if !task.collection {

			// if task.collection is not set, we execute the query as-is.
			// this will only work if the query returns exactly one row and one column.
			rows, err = tx.QueryContext(taskCtx, task.request)

		} else if task.geojson && task.pretty {

			// this generates prettified GeoJSON

			rows, err = tx.QueryContext(taskCtx, fmt.Sprintf(
				`SELECT jsonb_pretty(jsonb_build_object(
                    'type', 'FeatureCollection',
                    'properties', jsonb_build_object(
                       'timestamp', (select value from osm2pgsql_properties where property='replication_timestamp'),
                       'generator', 'Postpass API 0.2'
                       ),
                    'features', coalesce(jsonb_agg(ST_AsGeoJSON(t.*)::json), '[]'::jsonb)))
                FROM (%s) as t;`, task.request))

		} else if task.geojson && !task.pretty {

			// this generates un-prettified GeoJSON

			rows, err = tx.QueryContext(taskCtx, fmt.Sprintf(
				`SELECT json_build_object(
                    'type', 'FeatureCollection',
                    'properties', jsonb_build_object(
                       'timestamp', (select value from osm2pgsql_properties where property='replication_timestamp'),
                       'generator', 'Postpass API 0.2'
                       ),
                    'features', coalesce(jsonb_agg(ST_AsGeoJSON(t.*)::json), '[]'::jsonb))
                FROM (%s) as t;`, task.request))

		} else {

			// this collects results over multiple rows and columns,
			// but doesn't attempt to build GeoJSON

			rows, err = tx.QueryContext(taskCtx, fmt.Sprintf(
				`SELECT jsonb_pretty(jsonb_build_object(
                    'metadata', jsonb_build_object(
                       'timestamp', (select value from osm2pgsql_properties where property='replication_timestamp'),
                       'generator', 'Postpass API 0.2'
                       ),
                    'result', jsonb_agg(t.*)::jsonb))
                FROM (%s) as t;`, task.request))
		}

		if err != nil {
			task.response <- SqlResponse{err: true, result: err.Error()}
			Idle[id/100].Add(1)
			continue
		}

		// parse only one line of results
		rows.Next()

		// scan only one column of the result line
		err = rows.Scan(&res)

		// discard result
		rows.Close()

		if err != nil {
			task.response <- SqlResponse{err: true, result: err.Error()}
			Idle[id/100].Add(1)
			continue
		}

		// log.Printf("worker %d done\n", id)

		// send response back on channel
		task.response <- SqlResponse{err: false, result: res}
		Idle[id/100].Add(1)
	}
}
