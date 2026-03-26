package postpass

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"encoding/csv"
	"html"
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
		} else if task.output_format == "json" {
			res, err = json_output(db, taskCtx, task)
			content_type = "application/json"
		} else if task.output_format == "csv" {
			res, err = csv_output(db, taskCtx, task, ',', true)
			content_type = "text/csv"
		} else if task.output_format == "tsv" {
			res, err = csv_output(db, taskCtx, task, '\t', true)
			content_type = "text/tsv"
		} else if task.output_format == "csv_headerless" {
			res, err = csv_output(db, taskCtx, task, ',', false)
			content_type = "text/csv"
		} else if task.output_format == "tsv_headerless" {
			res, err = csv_output(db, taskCtx, task, '\t', false)
			content_type = "text/tsv"
		} else if task.output_format == "html_table" {
			res, err = html_table_output(db, taskCtx, task)
			content_type = "text/html"
		} else if task.output_format == "md_table" {
			res, err = markdown_table_output(db, taskCtx, task)
			content_type = "text/plain"
		} else {
			log.Printf("Impossible code path. Unsupported output_format %s", task.output_format)
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

func json_output(db *sql.DB, taskCtx context.Context, task WorkItem) (string, error) {
		// this executes the request on the database.
		var rows *sql.Rows
		var res string
		var err error

		var builder strings.Builder
		var line string
		row_num := 0

		// “header” of GeoJSON output

		builder.WriteString("[")

		// Now do each row
				
		rows, err = db.QueryContext(taskCtx, fmt.Sprintf(
			`SELECT to_json(t.*) FROM (%s) as t;`, task.request))

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
		builder.WriteString("]");

		if err != nil {
			return "", err
		}

		// discard result
		_ = rows.Close()

		res = builder.String()

		
		return res, err
}

func csv_output(db *sql.DB, taskCtx context.Context, task WorkItem, comma rune, show_header bool) (string, error) {
		// this executes the request on the database.
		var rows *sql.Rows
		var res string
		var err error

		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		writer.Comma = comma
		row_num := 0

		rows, err = db.QueryContext(taskCtx, task.request)

		if err != nil {
			return "", err
		}

		columns, err := rows.Columns()
		if err != nil {
			return "", err
		}
		if show_header {
			err = writer.Write(columns)
			if err != nil {
				return "", err
			}
		}
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}


		for rows.Next() {
			if err := rows.Scan(valuePtrs...); err != nil {
				return "", err
			}

			record := make([]string, len(columns))
			for i, val := range values {
				// Handle NULL values
				if val == nil {
					record[i] = ""
				} else {
					// Convert the value to a string
					record[i] = fmt.Sprintf("%v", val)
				}
			}

			// Write the record to the CSV
			if err := writer.Write(record); err != nil {
				return "", err
			}

			row_num ++
		}

		if err != nil {
			return "", err
		}

		// discard result
		_ = rows.Close()

		writer.Flush()
		if err := writer.Error() ; err != nil {
			return "", err
		}

		res = buf.String()
		
		return res, err
}

func html_table_output(db *sql.DB, taskCtx context.Context, task WorkItem) (string, error) {
		// this executes the request on the database.
		var rows *sql.Rows
		var res string
		var err error
		var builder strings.Builder


		builder.WriteString("<table>\n")

		rows, err = db.QueryContext(taskCtx, task.request)

		if err != nil {
			return "", err
		}

		columns, err := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		builder.WriteString("<thead>")
		for _, col := range columns {
			builder.WriteString("<th>")
			builder.WriteString(html.EscapeString(fmt.Sprintf("%v", col)))
			builder.WriteString("</th>")
		}
		builder.WriteString("</thead>\n")

		for rows.Next() {
			if err := rows.Scan(valuePtrs...); err != nil {
				return "", err
			}
			builder.WriteString("<tr>")

			for _, val := range values {
				builder.WriteString("<td>")
				// Handle NULL values
				if val != nil {
					builder.WriteString(html.EscapeString(fmt.Sprintf("%v", val)))
				}
				builder.WriteString("</td>")
			}
			builder.WriteString("</tr>\n")

		}
		builder.WriteString("</table>\n")

		if err != nil {
			return "", err
		}

		// discard result
		_ = rows.Close()

		res = builder.String()
		
		return res, err
}

func escapeMarkdown(s string) string {
	// Escape backticks, backslashes, and pipes
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}


func getColumnWidths(rows [][]string) []int {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	return widths
}

func formatMarkdownTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	// Calculate column widths
	widths := getColumnWidths(rows)

	// Build the table
	var sb strings.Builder

	// Write header row
	for i, cell := range rows[0] {
		sb.WriteString(fmt.Sprintf("| %-*s ", widths[i], cell))
	}
	sb.WriteString("|\n")

	// Write separator row
	for _, width := range widths {
		sb.WriteString(fmt.Sprintf("|-%s-", strings.Repeat("-", width)))
	}
	sb.WriteString("|\n")

	// Write data rows
	for _, row := range rows[1:] {
		for i, cell := range row {
			sb.WriteString(fmt.Sprintf("| %-*s ", widths[i], cell))
		}
		sb.WriteString("|\n")
	}

	return sb.String()
}

func markdown_table_output(db *sql.DB, taskCtx context.Context, task WorkItem) (string, error) {
		// this executes the request on the database.
		var rows *sql.Rows
		var table [][]string
		var err error
		var row []string

		rows, err = db.QueryContext(taskCtx, task.request)

		columns, err := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		row = row[:0]
		for _, col := range columns {
			row = append(row, escapeMarkdown(fmt.Sprintf("%v", col)))
		}
		table = append(table, row)

		for rows.Next() {
			if err := rows.Scan(valuePtrs...); err != nil {
				return "", err
			}
			row := make([]string, len(columns))

			for i, val := range values {
				if val == nil {
					row[i] = ""
				} else {
					row[i] = escapeMarkdown(fmt.Sprintf("%v", val))
				}
			}
			table = append(table, row)
		}
		// discard result
		_ = rows.Close()

		res := formatMarkdownTable(table)
		
		return res, err
}
