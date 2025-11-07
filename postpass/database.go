package postpass

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func explain(db *sql.DB, query string, queueOnly bool) ([]map[string]any, float64, float64, error) {
	var unparsedResult []byte
	var structuredParsedResult []struct {
		Plan struct {
			Startup float64 `json:"Startup Cost"`
			Total   float64 `json:"Total Cost"`
		} `json:"Plan"`
	}
	var unstructuredParsedResult []map[string]any

	// yes there is a possible SQL injection here but risk mitigation
	// must be done on PostgreSQL side - we do not want to build an SQL parser
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(fmt.Sprintf("EXPLAIN (FORMAT JSON) (%s)", query))

	if err != nil {
		return nil, 0, 0, err
	}

	// read only one row of EXPLAIN result
	rows.Next()

	// read only one column
	err = rows.Scan(&unparsedResult)

	// discard query
	defer rows.Close()

	// parse query costs
	err = json.Unmarshal(unparsedResult, &structuredParsedResult)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(structuredParsedResult) != 1 {
		return nil, 0, 0, fmt.Errorf("could not determine costs from explain output")
	}
	if !queueOnly {
		// parse full plan for json response
		err = json.Unmarshal(unparsedResult, &unstructuredParsedResult)
		if err != nil {
			return nil, 0, 0, err
		}
	}

	return unstructuredParsedResult, structuredParsedResult[0].Plan.Startup, structuredParsedResult[0].Plan.Total, nil
}
