package postpass

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

/*
 * API handler that receives a web request
 *
 * executes an EXPLAIN on the request
 * (which doubles as a syntax check)
 * and when EXPLAIN successful, sends the request to one of
 * three classes of worker.
 */
func HandleInterpreter(db *sql.DB, slow chan<- WorkItem, medium chan<- WorkItem, quick chan<- WorkItem, writer http.ResponseWriter, r *http.Request) {
	// create channel we want to receive the response on
	rchan := make(chan SqlResponse, 1)
	closeChan := make(chan struct{}, 1)
	defer close(closeChan)

	writer.Header().Set("Access-Control-Allow-Origin", "*")

	// process GET/POST parameters
	_ = r.ParseForm()
	tData := r.Form["data"]
	if tData == nil {
		log.Printf("no data field given\n")
		http.Error(writer, "no data field given", http.StatusBadRequest)
		return
	}
	data := tData[0]

	output_format := "geojson"
	found_valid_format := false
	allowed_output_formats := []string{"geojson", "geojson_w_props", "json", "csv", "csv_headerless", "tsv", "tsv_headerless", "html_table", "md_table", "sql_values", "with_sql_values"}
	wanted_output_format := r.Form["output_format"]
	if wanted_output_format != nil {
		wanted_output_format := wanted_output_format[0]
		for _, v := range allowed_output_formats {
			if wanted_output_format == v {
				output_format = v;
				found_valid_format =  true
			}
		}

		if ! found_valid_format {
			log.Printf("Output format of %s is not accepted\n", wanted_output_format)
			http.Error(writer, fmt.Sprintf("Output format of %s is not accepted", wanted_output_format), http.StatusBadRequest)
			return
		}
	}


	id := Count.Add(1)

	log.Printf("request #%d: query '%s' of='%s'\n", id,
		strings.Join(strings.Fields(strings.TrimSpace(data)), " "), output_format)

	var startTime = time.Now().UnixMilli()

	_, from, to, err := explain(db, data, true)
	if err != nil {
		log.Printf("request #%d: error in EXPLAIN: '%s'\n", id, err.Error())
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	// use average of two cost values given by EXPLAIN
	med := int((from + to) / 2)

	// create work item...
	work := WorkItem{
		request:       data,
		output_format: output_format,
		response:      rchan,
		closer:        closeChan,
	}

	// ... and send to appropriate channel
	if med < QuickMediumThreshold {
		log.Printf("request #%d: medium cost is %d, sending to quick worker\n", id, med)
		quick <- work
	} else if med < MediumSlowThreshold {
		log.Printf("request #%d: medium cost is %d, sending to medium worker\n", id, med)
		medium <- work
	} else {
		log.Printf("request #%d: medium cost is %d, sending to slow worker\n", id, med)
		slow <- work
	}

	var rv SqlResponse

	// wait for response
	select {
	case rv = <-rchan:
	case <-r.Context().Done():
		closeChan <- struct{}{}
		log.Printf("request #%d: client hung up before query got completed\n", id)
		return
	}

	var elapsed = time.Now().UnixMilli() - startTime

	// and send response to HTTP client
	if rv.err {
		// FIXME it isn't really a bad request if it fails here, is it?
		log.Printf("request #%d: error from database after %dms: '%s'\n",
			id, elapsed, rv.result)
		http.Error(writer, rv.result, http.StatusBadRequest)
	}

	writer.Header().Set("Content-Type", rv.content_type)

	log.Printf("request #%d: completed after %dms, response size is %d\n",
		id, elapsed, len(rv.result))
	_,_  = fmt.Fprintf(writer, "%s", rv.result)
}

func HandleExplain(db *sql.DB, writer http.ResponseWriter, r *http.Request) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Content-Type", "application/json")

	// process GET/POST parameters
	_  = r.ParseForm()
	tData := r.Form["data"]
	if tData == nil {
		log.Printf("no data field given\n")
		http.Error(writer, "no data field given", http.StatusBadRequest)
		return
	}
	data := tData[0]

	log.Printf("explain request: query '%s'\n",
		strings.Join(strings.Fields(strings.TrimSpace(data)), " "))

	var startTime = time.Now().UnixMilli()

	full, from, to, err := explain(db, data, false)
	if err != nil {
		log.Printf("explain request: error in EXPLAIN: '%s'\n", err.Error())
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	// use average of two cost values given by EXPLAIN
	med := int((from + to) / 2)

	response := map[string]any{"plan": full}

	// ... and send the queue decision back to the client
	if med < QuickMediumThreshold {
		response["queue"] = "quick"
	} else if med < MediumSlowThreshold {
		response["queue"] = "medium"
	} else {
		response["queue"] = "slow"
	}

	err = json.NewEncoder(writer).Encode(response)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}

	//fmt.Fprint(writer, "slow")
	log.Printf("explain request: completed after %dms\n", time.Now().UnixMilli()-startTime)
}
