package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
)

type MetadataInfo struct {
	FlattenedFields []string `json:"flattened_fields"`
}

func createTableFromJSON(db *sql.DB, jsonData interface{}) error {
	switch data := jsonData.(type) {
	case []interface{}:
		if len(data) == 0 {
			return fmt.Errorf("JSON array is empty")
		}
		// Assuming the first item in the slice gives us the structure of the JSON data
		firstItem, ok := data[0].(map[string]interface{})
		if !ok {
			return fmt.Errorf("unexpected structure: expected map, got %T", data[0])
		}
		return createTableForMap(db, firstItem)
	case map[string]interface{}:
		// If it's a single JSON object
		return createTableForMap(db, data)
	default:
		return fmt.Errorf("unsupported JSON structure: %T", data)
	}
}

func createTableForMap(db *sql.DB, jsonMap map[string]interface{}) error {
	createStmt := "CREATE TABLE IF NOT EXISTS json_data ("
	for key, value := range jsonMap {
		// Determine the data type of each field
		fieldType := determineFieldType(value)
		createStmt += fmt.Sprintf("%s %s,", key, fieldType)
	}

	// Remove the last comma and close the statement
	createStmt = strings.TrimRight(createStmt, ",") + ");"
	_, err := db.Exec(createStmt)
	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}
	return nil
}

func determineFieldType(value interface{}) string {
	switch reflect.TypeOf(value).Kind() {
	case reflect.String:
		return "TEXT"
	case reflect.Float64:
		return "DOUBLE"
	case reflect.Int, reflect.Int64:
		return "INTEGER"
	case reflect.Bool:
		return "BOOLEAN"
	case reflect.Map, reflect.Slice:
		// For nested objects or arrays, store them as serialized JSON strings
		return "TEXT"
	default:
		return "TEXT" // Fallback to TEXT for unsupported types
	}
}

func insertMapIntoDuckDB(db *sql.DB, jsonMap map[string]interface{}) error {
	// Build SQL INSERT INTO statement dynamically
	columns := ""
	values := ""
	args := []interface{}{}
	for key, value := range jsonMap {
		columns += key + ","
		values += "?,"

		// Ensure the value is not nil before checking its type
		if value == nil {
			// Handle nil values (e.g., store as NULL or empty string)
			args = append(args, nil)
			continue
		}

		switch reflect.TypeOf(value).Kind() {
		case reflect.Map, reflect.Slice:
			// Serialize nested structures to JSON
			serializedValue, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to serialize nested structure for key %s: %v", key, err)
			}
			args = append(args, string(serializedValue)) // Store as string
		default:
			args = append(args, value)
		}
	}

	// Remove the last comma and build the final query
	columns = strings.TrimRight(columns, ",")
	values = strings.TrimRight(values, ",")
	insertStmt := fmt.Sprintf("INSERT INTO json_data (%s) VALUES (%s);", columns, values)

	// Execute the query
	_, err := db.Exec(insertStmt, args...)
	if err != nil {
		return fmt.Errorf("failed to insert data: %v", err)
	}
	return nil
}

func insertDataIntoDuckDB(db *sql.DB, jsonData interface{}) error {
	switch data := jsonData.(type) {
	case []interface{}:
		for i, item := range data {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				// Log the error but continue with the next entry
				log.Printf("Skipping entry %d: unexpected type in JSON array, expected map[string]interface{}, got %T", i, item)
				continue
			}
			err := insertMapIntoDuckDB(db, itemMap)
			if err != nil {
				// Log the error but continue with the next entry
				log.Printf("Error inserting entry %d into DuckDB: %v", i, err)
				continue
			}
		}
		return nil
	case map[string]interface{}:
		return insertMapIntoDuckDB(db, data)
	default:
		return fmt.Errorf("unsupported JSON structure for insertion: %T", jsonData)
	}
}

func queryDuckDB(db *sql.DB, query string) ([]map[string]interface{}, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := []map[string]interface{}{}
	for rows.Next() {
		// Create a slice of interfaces to hold the row data
		row := make([]interface{}, len(columns))
		rowPointers := make([]interface{}, len(columns))
		for i := range row {
			rowPointers[i] = &row[i]
		}
		err := rows.Scan(rowPointers...)
		if err != nil {
			return nil, err
		}

		// Convert row into a map
		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = row[i]
		}
		results = append(results, rowMap)
	}

	return results, nil
}

func getTableSchema(db *sql.DB, tableName string) (string, error) {
	query := fmt.Sprintf("PRAGMA table_info('%s');", tableName)
	rows, err := db.Query(query)
	if err != nil {
		return "", fmt.Errorf("failed to get table schema: %v", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var schema []map[string]interface{}
	var schemaText string
	for rows.Next() {
		row := make([]interface{}, len(columns))
		rowPointers := make([]interface{}, len(columns))
		for i := range row {
			rowPointers[i] = &row[i]
		}
		err := rows.Scan(rowPointers...)
		if err != nil {
			return "", err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			rowMap[colName] = row[i]
		}
		schema = append(schema, rowMap)

		schemaText += fmt.Sprintf("%v %v, ", rowMap["name"], rowMap["type"])
	}

	schemaText = strings.TrimRight(schemaText, ", ")
	return schemaText, nil
}
