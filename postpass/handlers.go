package postpass

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
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
func HandleInterpreter(
	db *sql.DB,
	slow chan<- WorkItem,
	medium chan<- WorkItem,
	quick chan<- WorkItem,
	cfg *PostpassConfig,
	writer http.ResponseWriter,
	r *http.Request,
	metrics *Metrics,
) {
	// create channel we want to receive the response on
	rchan := make(chan SqlResponse, 1)
	closeChan := make(chan struct{}, 1)
	defer close(closeChan)

	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Server", "Postpass API 0.2")

	// process GET/POST parameters
	// Prefer q= then data=
	_ = r.ParseForm()
	query := ""
	if values, ok := r.Form["q"]; ok {
		query = values[0]
	} else if values, ok := r.Form["data"]; ok {
		query = values[0]
	} else {
		log.Printf("no q/data field given\n")
		http.Error(writer, "no query field given", http.StatusBadRequest)
		return
	}

	// Append a newline at the end to escape single line comments
	query = query + "\n"

	geojson := true
	tGeojson := r.Form["options[geojson]"]
	if tGeojson != nil {
		geojson, _ = strconv.ParseBool(tGeojson[0])
	}

	if r.Method == http.MethodGet {
		tCacheFor := r.Form["cache_for"]
		if tCacheFor == nil {
			tCacheFor = r.Form["options[cache_for]"]
		}
		if tCacheFor != nil {
			parsed, _ := strconv.ParseUint(tCacheFor[0], 10, 64)
			cache_for := uint(parsed)
			// 2 days is 172,800. Longer caching is v. unlikely to be needed.
			if cache_for > 172800 {
				cache_for = 172800
			}

			writer.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", cache_for))
		}
	}

	id := Count.Add(1)

	log.Printf("request #%d: query '%s' g=%t\n", id,
		strings.Join(strings.Fields(strings.TrimSpace(query)), " "),
		geojson)

	var startTime = time.Now()

	_, from, to, err := explain(db, query, true)
	if err != nil {
		log.Printf("request #%d: error in EXPLAIN: '%s'\n", id, err.Error())
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	metrics.ReqRecv.Inc()

	// use average of two cost values given by EXPLAIN
	med := int((from + to) / 2)

	// create work item...
	work := WorkItem{
		request:     query,
		geojson:     geojson,
		response:    rchan,
		closer:      closeChan,
		when_queued: startTime,
		est_cost:    med,
	}

	// ... and send to appropriate channel
	if med < cfg.QuickMediumThreshold {
		log.Printf("request #%d: medium cost is %d, sending to quick worker\n", id, med)
		work.queue = "quick"
		quick <- work
	} else if med < cfg.MediumSlowThreshold {
		log.Printf("request #%d: medium cost is %d, sending to medium worker\n", id, med)
		work.queue = "medium"
		medium <- work
	} else {
		log.Printf("request #%d: medium cost is %d, sending to slow worker\n", id, med)
		work.queue = "slow"
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

	var elapsed = time.Now().UnixMilli() - startTime.UnixMilli()

	// and send response to HTTP client
	if rv.err {
		// FIXME it isn't really a bad request if it fails here, is it?
		log.Printf("request #%d: error from database after %dms: '%s'\n",
			id, elapsed, rv.result)
		http.Error(writer, rv.result, http.StatusBadRequest)
	}

	log.Printf("request #%d: completed after %dms, response size is %d\n",
		id, elapsed, len(rv.result))
	_, _ = fmt.Fprintf(writer, "%s", rv.result)
}

func HandleExplain(db *sql.DB, cfg *PostpassConfig, writer http.ResponseWriter, r *http.Request) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Content-Type", "application/json")

	// process GET/POST parameters
	_ = r.ParseForm()
	tData := r.Form["data"]
	if tData == nil {
		log.Printf("no data field given\n")
		http.Error(writer, "no data field given", http.StatusBadRequest)
		return
	}

	// Append a newline at the end to escape single line comments
	query := tData[0] + "\n"

	log.Printf("explain request: query '%s'\n",
		strings.Join(strings.Fields(strings.TrimSpace(query)), " "))

	var startTime = time.Now().UnixMilli()

	full, from, to, err := explain(db, query, false)
	if err != nil {
		log.Printf("explain request: error in EXPLAIN: '%s'\n", err.Error())
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	// use average of two cost values given by EXPLAIN
	med := int((from + to) / 2)

	response := map[string]any{"plan": full}

	// ... and send the queue decision back to the client
	if med < cfg.QuickMediumThreshold {
		response["queue"] = "quick"
	} else if med < cfg.MediumSlowThreshold {
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
