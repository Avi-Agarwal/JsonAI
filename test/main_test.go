package test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lpernett/godotenv"
	_ "github.com/marcboeker/go-duckdb"
	"github.com/sashabaranov/go-openai"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"
	"testing"
)

const (
	invalidQuestionResponse = "I cannot answer the query using the information from the file"
	maxResultTokenLimit     = 18000
	maxTokenLimit           = 25000
)

// Function to estimate the number of tokens in a string
func estimateTokenCount(content string) int {
	return len(strings.Split(content, " "))
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

func downloadFileFromS3(bucket, key string) (string, error) {
	// S3 Configurations
	accessKeyID, _ := os.LookupEnv("JAI_AWS_ACCESS_KEY")
	secretAccessKey, _ := os.LookupEnv("JAI_AWS_SECRET_KEY")
	awsRegion := "us-east-2"
	//bucketName := "json-ai"

	creds := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""))

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsRegion),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		log.Printf("Error loading AWS config: %v", err)
		return "", err
	}

	// Create a new S3 client
	client := s3.NewFromConfig(cfg)

	// Get the file from S3
	result, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("unable to get object from S3: %v", err)
	}
	defer result.Body.Close()

	// Read the file content
	body, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %v", err)
	}

	return string(body), nil
}

// Get the OpenAI chat response
func GetOpenAIChatResponse(messages []openai.ChatCompletionMessage) (string, error) {
	client := openai.NewClient(os.Getenv("JAI_OPENAI_KEY"))
	stream, err := client.CreateChatCompletionStream(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    "gpt-4o",
			Messages: messages,
			Stream:   true,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create stream: %v", err)
	}
	defer stream.Close()

	var fullResponse string
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		fullResponse += response.Choices[0].Delta.Content
	}

	return fullResponse, nil
}

func getJSONPreview(jsonContent string, previewLength int) string {
	if len(jsonContent) > previewLength {
		return jsonContent[:previewLength] + "..." // Add ellipsis to indicate truncation
	}
	return jsonContent
}

// Insert JSON data into DuckDB
func insertDataIntoDuckDB2(db *sql.DB, jsonData interface{}) error {
	// Assuming jsonData is a slice of map[string]interface{}
	switch reflect.TypeOf(jsonData).Kind() {
	case reflect.Slice:
		for _, item := range jsonData.([]interface{}) {
			err := insertMapIntoDuckDB(db, item.(map[string]interface{}))
			if err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported JSON structure for insertion")
	}
}

// Run a query on DuckDB
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

func getTableSchemaOld(db *sql.DB, tableName string) ([]map[string]interface{}, error) {
	query := fmt.Sprintf("PRAGMA table_info('%s');", tableName)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get table schema: %v", err)
	}
	defer rows.Close()

	// Retrieve column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Prepare a slice to hold the schema data
	var schema []map[string]interface{}

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
		schema = append(schema, rowMap)
	}

	return schema, nil
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

// Helper function to clean SQL output by removing backticks and 'sql' wrapper
func cleanSQLGeneration(sqlQuery string) string {
	// Check if the output is wrapped with ```sql or similar patterns
	sqlQuery = strings.TrimSpace(sqlQuery)

	// Remove possible wrapping ```sql or ``` around the SQL query
	sqlQuery = strings.TrimPrefix(sqlQuery, "```sql")
	sqlQuery = strings.TrimSuffix(sqlQuery, "```")

	// Return the cleaned query
	return strings.TrimSpace(sqlQuery)
}

// Generate SQL query from OpenAI
func openAIChat(messages *[]openai.ChatCompletionMessage) (string, error) {
	client := openai.NewClient(os.Getenv("JAI_OPENAI_KEY"))
	response, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: *messages,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get response: %v", err)
	}
	*messages = append(*messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: response.Choices[0].Message.Content})

	return response.Choices[0].Message.Content, nil
}

func genericResultsToString(results []map[string]interface{}) string {
	var resultsString string
	for _, result := range results {
		var resultParts []string
		for key, value := range result {
			resultParts = append(resultParts, fmt.Sprintf("%s: %v", key, value))
		}
		resultsString += strings.Join(resultParts, ", ") + "\n"
	}

	return resultsString
}

func convertQuestionToSQLAndRetrieveResults(db *sql.DB, userQuery, tableName, schema, jsonPreview string) (string, string, error) {
	// Retrieve the schema of the 'json_data' table
	sqlGenMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are an AI assistant that generates SQL queries for DuckDB. The results of the query you produce will be fed back into OpenAI to answer the user's original question. Please ensure the result of the query is limited to a reasonable size to avoid hitting token limits. Let's aim for results that would take 1000 or less openAI tokens"},
	}

	// Create the initial prompt for OpenAI to generate the SQL query
	prompt := fmt.Sprintf(`Based on the user's question: "%s", generate a SQL query that will answer the question or a SQL query that will give us the information needed to answer the question. The table schema for the JSON data is as follows:
Table Name: %s
Schema: %s

Here is a small preview of the actual JSON data that was used to create the table:
%s

Only Generate the SQL query as your output, without any explanations. I will directly take the response you generate as SQL and run it on the DuckDB database to get the data needed to answer the user's question. I'll feed the data from running the SQL back to an LLM to answer the original question'.
`, userQuery, tableName, schema, jsonPreview)

	// Append the prompt to the message array
	sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt})

	tries := 10
	retrievedData := false
	var results []map[string]interface{}
	var finalSQLQuery string

	for tries > 0 && !retrievedData {
		// Generate the SQL query from OpenAI
		sqlQuery, err := openAIChat(&sqlGenMessages)
		if err != nil {
			return "", "", fmt.Errorf("Failed to generate SQL query: %v", err)
		}
		sqlQuery = cleanSQLGeneration(sqlQuery)

		fmt.Printf("Generated SQL: %s\n", sqlQuery)

		// Try to execute the SQL query
		results, err = queryDuckDB(db, sqlQuery)
		if err != nil {
			fmt.Printf("Error executing SQL query: %v\n", err)
			errorMessage := fmt.Sprintf("The query you generated: '%s' resulted in the following error: %v\n Please fix the query.", sqlQuery, err)

			sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: errorMessage})
		} else {
			retrievedData = true
			finalSQLQuery = sqlQuery // Save final query
		}

		tries--
	}

	// If no valid results were retrieved after 10 tries, return an error
	if !retrievedData {
		return "", "", fmt.Errorf("Failed to generate a valid SQL query after 10 attempts.")
	}

	// Return the results and the final SQL query used
	return genericResultsToString(results), finalSQLQuery, nil
}

func retrieveRelevantInformation(db *sql.DB, userQuery, tableName, schema, jsonPreview string) (string, string, error) {
	// Retrieve the schema of the 'json_data' table
	sqlGenMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are an AI assistant that generates SQL queries for DuckDB. The results of the query you produce will be fed back into OpenAI to answer the user's original question. Please ensure the result of the query is limited to a reasonable size to avoid hitting token limits. Let's aim for results that would take 1000 or less openAI tokens."},
	}

	// Create the initial prompt for OpenAI to generate the SQL query
	prompt := fmt.Sprintf(`Based on the user's question: "%s", generate a SQL query that will give us the relevant information from the large JSON file needed to answer the user's question. The table schema for the JSON data is as follows:
Table Name: %s
Schema: %s

Here is a small preview of the actual JSON data that was used to create the table:
%s

Only Generate the SQL query as your output, without any explanations. I will directly take the response you generate as SQL and run it on the DuckDB database to get the data needed to answer the user's question. I'll feed the data from running the SQL back to an LLM to answer the original question'.
`, userQuery, tableName, schema, jsonPreview)

	// Append the prompt to the message array
	sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt})

	tries := 10
	retrievedData := false
	var results []map[string]interface{}
	var finalSQLQuery string

	for tries > 0 && !retrievedData {
		// Generate the SQL query from OpenAI
		sqlQuery, err := openAIChat(&sqlGenMessages)
		if err != nil {
			return "", "", fmt.Errorf("Failed to generate SQL query: %v", err)
		}
		sqlQuery = cleanSQLGeneration(sqlQuery)

		fmt.Printf("Generated SQL: %s\n", sqlQuery)

		// Try to execute the SQL query
		results, err = queryDuckDB(db, sqlQuery)
		if err != nil {
			fmt.Printf("Error executing SQL query: %v\n", err)
			errorMessage := fmt.Sprintf("The query you generated: '%s' resulted in the following error: %v\n Please fix the query.", sqlQuery, err)

			sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: errorMessage})
		} else {
			retrievedData = true
			finalSQLQuery = sqlQuery // Save final query
		}

		tries--
	}

	// If no valid results were retrieved after 10 tries, return an error
	if !retrievedData {
		return "", "", fmt.Errorf("Failed to generate a valid SQL query after 10 attempts.")
	}

	// Return the results and the final SQL query used
	return genericResultsToString(results), finalSQLQuery, nil
}

// Function to send results back to OpenAI to answer the user's question based on query results
func askOpenAIToAnswerBasedOnResults(results string, userQuestion string) (string, error) {
	// Create the conversation context for OpenAI
	answerMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `You are an AI assistant that helps users answer questions by analyzing large JSON data. The system processes large JSON files by loading them into a database, executing queries, and retrieving results. Your role is to analyze the query results and answer the user's original question based on the data retrieved from the database.`},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(`We received a large JSON from the user. We put the large JSON into a database and ran some queries. The following is the queries run and their results:
%s

Now, using this information, please answer the user's original question in a kind and friendly way. Do not mention the database or query in your response. Please answer as if you knew this information and are simply answering the users question:
%s`, results, userQuestion)},
	}

	// Send the conversation to OpenAI and get the answer
	answer, err := openAIChat(&answerMessages)
	if err != nil {
		return "", fmt.Errorf("Failed to get an answer from OpenAI: %v", err)
	}

	return strings.TrimSpace(answer), nil
}

func validateUserQuestion(userQuestion, schema, jsonPreview string) (bool, error) {
	validationMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `You are an AI assistant that determines if a user's question can be answered using the given JSON data preview and schema.`},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(`Here is a preview of the JSON data:
%s

And here is the schema:
%s

The user has asked the following question:
"%s"

Please return '1' if the question is relevant and can be answered using the JSON data, or '0' if it cannot be answered. Make sure to only return a single char, '1' or '0'.
`, jsonPreview, schema, userQuestion)},
	}

	response, err := openAIChat(&validationMessages)
	if err != nil {
		return false, fmt.Errorf("Failed to validate user question with OpenAI: %v", err)
	}

	return response == "1", nil
}

func TestMainExperiment(t *testing.T) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	userQuery := "What are the names of the people in this file?"

	//userQuery := "What is the most common nationality? "

	// Load the JSON file
	bucket := "json-ai"
	key := "64fe3752-28cd-4208-9105-bb62f20dc9c3-member_info.json"

	// Download the JSON file from S3
	jsonContent, err := downloadFileFromS3(bucket, key)
	if err != nil {
		log.Fatalf("Error downloading JSON file: %v", err)
	}

	jsonPreview := getJSONPreview(jsonContent, 2000)

	// Parse the JSON file into a generic structure
	var jsonData interface{}
	err = json.Unmarshal([]byte(jsonContent), &jsonData)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	// Open a DuckDB in-memory database
	db, err := sql.Open("duckdb", "")
	if err != nil {
		log.Fatalf("Failed to open DuckDB: %v", err)
	}
	defer db.Close()

	// Create the table based on the JSON structure
	err = createTableFromJSON(db, jsonData)
	if err != nil {
		log.Fatalf("Failed to create table in DuckDB: %v", err)
	}

	// Insert the JSON data into the table
	err = insertDataIntoDuckDB(db, jsonData)
	if err != nil {
		log.Fatalf("Failed to insert data into DuckDB: %v", err)
	}

	// Retrieve the schema of the 'json_data' table
	schema, err := getTableSchema(db, "json_data")
	if err != nil {
		log.Fatalf("Failed to get table schema: %v", err)
	}
	tableName := "json_data"

	// Function to validate if the users question is valid and can be answered given the data
	isValidQuestion, err := validateUserQuestion(userQuery, schema, jsonPreview)
	if err != nil {
		log.Fatalf("Error validating user question: %v", err)
	}

	if !isValidQuestion {
		fmt.Printf("\nQuestion: %s\nFinal Answer: %s\n", userQuery, invalidQuestionResponse)
		return
	}

	resultsString := ""

	// Run the SQL query and retrieve results
	results, sqlQuery, err := convertQuestionToSQLAndRetrieveResults(db, userQuery, tableName, schema, jsonPreview)
	if err != nil {
		log.Fatalf("Error running SQL query: %v", err)
	}
	resultsString = resultsString + fmt.Sprintf("Query Run: %s\nQuery Results: %s\n", sqlQuery, results)

	// Estimate the token usage after the first result
	result1EstimatedTokens := estimateTokenCount(resultsString)

	// If estimated tokens exceed the max limit, skip the second query
	if result1EstimatedTokens >= maxResultTokenLimit {
		log.Printf("Skipping second query because the first result is already too large. Estimated tokens: %d", result1EstimatedTokens)
	} else {
		// Run the second query and append its result
		results, sqlQuery, err = retrieveRelevantInformation(db, userQuery, tableName, schema, jsonPreview)
		if err != nil {
			log.Fatalf("Error running SQL query: %v", err)
		}

		// Include the query run information in the token count
		result2String := fmt.Sprintf("\nQuery Run: %s\nQuery Results: %s\n", sqlQuery, results)
		result2EstimatedTokens := estimateTokenCount(result2String)

		// If the combined token usage is under the limit, append the second result
		if result1EstimatedTokens+result2EstimatedTokens <= maxResultTokenLimit {
			resultsString = resultsString + result2String
		} else {
			log.Printf("Skipping appending second result as it exceeds the token limit. Total tokens: %d", result1EstimatedTokens+result2EstimatedTokens)
		}
	}

	// Send the results back to OpenAI for a final answer
	finalAnswer, err := askOpenAIToAnswerBasedOnResults(resultsString, userQuery)
	if err != nil {
		log.Fatalf("Error getting answer from OpenAI: %v", err)
	}

	// Output the final answer
	fmt.Printf("\nQuestion: %s\nFinal Answer: %s\n", userQuery, finalAnswer)
	return
}
