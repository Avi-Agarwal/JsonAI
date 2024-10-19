package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
)

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
	columns := ""
	values := ""
	args := []interface{}{}
	for key, value := range jsonMap {
		columns += key + ","
		values += "?,"

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
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}(rows)

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

func flattenJSONFields(jsonData map[string]interface{}, prefix string, column string, uniqueFields map[string]map[string]struct{}) {
	for key, value := range jsonData {
		newKey := key
		if prefix != "" {
			newKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			// Recursively flatten nested maps
			flattenJSONFields(v, newKey, column, uniqueFields)
		default:
			// Add the flattened key to the unique fields map, grouped by column
			if _, ok := uniqueFields[column]; !ok {
				uniqueFields[column] = make(map[string]struct{})
			}
			uniqueFields[column][newKey] = struct{}{} // Ensure the key is unique
		}
	}
}

// Function to check if a string contains valid JSON
func isValidJSON(str string) bool {
	var js map[string]interface{}
	err := json.Unmarshal([]byte(str), &js)
	return err == nil
}

// Function to dynamically detect JSON columns and extract unique nested fields
func extractUniqueFieldsFromJSONColumns(db *sql.DB, tableName string) (string, error) {
	uniqueFields := make(map[string]map[string]struct{})
	query := fmt.Sprintf("PRAGMA table_info(%s);", tableName)

	// Query the table schema to identify columns
	rows, err := db.Query(query)
	if err != nil {
		return "", fmt.Errorf("failed to query table schema: %v", err)
	}
	defer rows.Close()

	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name, colType, notnull, pk string
		var dfltValue sql.NullString // Handling potential NULL for dflt_value

		// Adjust the scan to use sql.NullString for nullable fields
		err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk)
		if err != nil {
			log.Printf("Failed to scan table schema: %v", err)
			continue
		}

		// Add VARCHAR or TEXT columns that could contain JSON to the list
		if colType == "VARCHAR" || colType == "TEXT" {
			columns = append(columns, name)
		}
	}

	// Iterate over JSON-like columns to extract and flatten fields
	for _, col := range columns {
		query := fmt.Sprintf("SELECT %s FROM %s;", col, tableName)
		rows, err := db.Query(query)
		if err != nil {
			log.Printf("Failed to query column %s: %v", col, err)
			continue
		}
		defer rows.Close()

		// Process each row in the column
		for rows.Next() {
			var jsonStr string
			err := rows.Scan(&jsonStr)
			if err != nil {
				log.Printf("Failed to scan row for column %s: %v", col, err)
				continue
			}

			// Check if the string is valid JSON
			if !isValidJSON(jsonStr) {
				continue
			}

			// Attempt to parse the JSON string
			var jsonData map[string]interface{}
			err = json.Unmarshal([]byte(jsonStr), &jsonData)
			if err != nil {
				// If it's not valid JSON, skip this row
				log.Printf("Invalid JSON in column %s: %v", col, err)
				continue
			}

			// Flatten the JSON and track unique fields
			flattenJSONFields(jsonData, "", col, uniqueFields)
		}
	}

	// Build a structured string of unique fields by column
	var result strings.Builder
	for col, fields := range uniqueFields {
		result.WriteString(fmt.Sprintf("Column: %s\n", col))
		for field := range fields {
			result.WriteString(fmt.Sprintf("  - %s\n", field))
		}
	}

	return result.String(), nil
}
