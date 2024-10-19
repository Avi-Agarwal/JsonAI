package server

import (
	"database/sql"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"log"
	"strings"
)

func (s Server) ConvertUserQuestionToSQLAndRetrieveQueryResults(duckDB *sql.DB, userQuestion, tableName, schema, jsonPreview string) (string, string, error) {
	sqlGenMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are an AI assistant that generates SQL queries for DuckDB. The results of the query you produce will be fed back into OpenAI to answer the user's original question. Please ensure the result of the query is limited to a reasonable size to avoid hitting openAIs token limits. Let's aim for the results of the query to be 1000 or less openAI tokens"},
	}

	prompt := fmt.Sprintf(`Based on the user's question: "%s", generate a SQL query that will answer the question or a SQL query that will give us the information needed to answer the question. The table schema for the JSON data is as follows:
Table Name: %s
Schema: %s

Here is a small preview of the actual JSON data that was used to create the table:
%s

Only Generate the SQL query as your output, without any explanations. I will directly take the response you generate as SQL and run it on the DuckDB database to get the data needed to answer the user's question. I'll feed the data from running the SQL back to an LLM to answer the original question'.
`, userQuestion, tableName, schema, jsonPreview)

	// Append the prompt to the message array
	sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt})

	tries := 10
	retrievedData := false
	var results []map[string]interface{}
	var finalSQLQuery string

	for tries > 0 && !retrievedData {
		// Generate the SQL query from OpenAI
		sqlQuery, err := s.OpenAIChat(&sqlGenMessages)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate SQL query: %v", err)
		}
		sqlQuery = cleanSQLGeneration(sqlQuery)

		fmt.Printf("Generated SQL: %s\n", sqlQuery)

		// Try to execute the SQL query
		results, err = queryDuckDB(duckDB, sqlQuery)
		if err != nil {
			fmt.Printf("Error executing SQL query: %v\n", err)
			errorMessage := fmt.Sprintf("The query you generated: '%s' resulted in the following error: %v\n Please fix the query.", sqlQuery, err)

			sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: errorMessage})
		} else {
			retrievedData = true
			finalSQLQuery = sqlQuery
		}

		tries--
	}

	if !retrievedData {
		return "", "", fmt.Errorf("failed to generate a valid SQL query after 10 attempts")
	}

	return genericResultsToString(results), finalSQLQuery, nil
}

func (s Server) RetrieveRelevantInformation(db *sql.DB, userQuestion, tableName, schema, jsonPreview string) (string, string, error) {
	sqlGenMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are an AI assistant that generates SQL queries for DuckDB. The results of the query you produce will be fed back into OpenAI to answer the user's original question. Please ensure the result of the query is limited to a reasonable size to avoid hitting token limits. Let's aim for results that would take 1000 or less openAI tokens"},
	}

	prompt := fmt.Sprintf(`Based on the user's question: "%s", generate a SQL query that will give us the relevant information from the large JSON file needed to answer the user's question. The table schema for the JSON data is as follows:
Table Name: %s
Schema: %s

Here is a small preview of the actual JSON data that was used to create the table:
%s

Only Generate the SQL query as your output, without any explanations. I will directly take the response you generate as SQL and run it on the DuckDB database to get the data needed to answer the user's question. I'll feed the data from running the SQL back to an LLM to answer the original question'.
`, userQuestion, tableName, schema, jsonPreview)

	// Append the prompt to the message array
	sqlGenMessages = append(sqlGenMessages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt})

	tries := 10
	retrievedData := false
	var results []map[string]interface{}
	var finalSQLQuery string

	for tries > 0 && !retrievedData {
		// Generate the SQL query from OpenAI
		sqlQuery, err := s.OpenAIChat(&sqlGenMessages)
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
			finalSQLQuery = sqlQuery
		}
		tries--
	}

	// If no valid results were retrieved after 10 tries, return an error
	if !retrievedData {
		return "", "", fmt.Errorf("failed to generate a valid SQL query after 10 attempts")
	}

	return genericResultsToString(results), finalSQLQuery, nil
}

func (s Server) AnswerUserQuestionBasedOnSQlResults(results string, userQuestion string) (string, error) {
	answerMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `You are an AI assistant that helps users answer questions by analyzing large JSON data. The system processes large JSON files by loading them into a database, executing queries, and retrieving results. Your role is to analyze the query results and answer the user's original question based on the data retrieved from the database.`},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(`We received a large JSON from the user. We put the large JSON into a database and ran some queries. The following is the queries run and their results:
%s

Now, using this information, please answer the user's original question in a kind and friendly way. Do not mention the database or query in your response. Please answer as if you knew this information and are simply answering the users question:
%s`, results, userQuestion)},
	}

	// Send the conversation to OpenAI and get the answer
	answer, err := s.OpenAIChat(&answerMessages)
	if err != nil {
		return "", fmt.Errorf("failed to get an answer from OpenAI: %v", err)
	}

	return strings.TrimSpace(answer), nil
}

func (s Server) ValidateUserQuestion(userQuestion, schema, jsonPreview, jsonName string) (bool, error) {
	validationMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `You are an AI assistant tasked with determining whether a user's question can be answered, inferred, or at least attempted using a given JSON file. You will be provided a schema and a preview of the data. Your task is to determine if the question relates to the data, either directly or indirectly, by matching key terms in the question to fields in the JSON or making logical inferences. If a term from the question does not match exactly, but there is a closely related field, you should still consider it as relevant and infer a connection.`},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(`
The JSON file is called "%s". Below is a preview of the JSON data:
%s

Here is the schema of the DuckDB table the JSON has been loaded into:
%s

The user has asked the following question about the JSON:
"%s"

Please determine whether this question could be answered, inferred, or at least attempted based on the JSON. Even if the exact terms don't match, make logical inferences when possible (e.g., consider synonyms or related fields). The relevant data could be nested within fields like 'metadata', be sure to consider those details as well. 

Err toward returning '1' unless the question is completely unrelated, and you cannot even make a reasonable attempt to answer it.

If you are leaning 0, ask yourself, given the schema and preview JSON, can you come up with a SQL that could explore this data to potentially answer the users question? Only if you cannot even come up with a SQL query to explore the data to answer the question, only then return 0.

Return '1' if you believe the question could be answered, inferred, attempted, or if you can even guess at the answer. Return '0' if you are certain the data is completely irrelevant to the question. If you return 0, please tell me the reason why.
`, jsonName, jsonPreview, schema, userQuestion)},
	}

	response, err := s.OpenAIChat(&validationMessages)
	if err != nil {
		return false, fmt.Errorf("failed to validate user question with OpenAI: %v", err)
	}

	if response != "1" {
		log.Printf("User question validation response: %s", response)
	}

	return response == "1", nil
}

func (s Server) ValidateUserQuestionBasedOnJson(userQuestion, jsonContent string) (bool, error) {
	validationMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `You are an AI assistant tasked with determining whether a user's question can be answered, inferred, or at least attempted using a given JSON file. You will be provided the full JSON file. Your task is to determine if the question relates to the data, either directly or indirectly, by matching key terms in the question to fields in the JSON or making logical inferences. If a term from the question does not match exactly, but there is a closely related field, you should still consider it as relevant and infer a connection.`},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(`
Here is the JSON file:
%s

The user has asked the following question about the JSON:
"%s"

Please determine whether this question could be answered, inferred, or at least attempted based on the JSON. Even if the exact terms don't match, make logical inferences when possible (e.g., consider synonyms or related fields).

Err toward returning '1' unless the question is completely unrelated, and you cannot even make a reasonable attempt to answer it.

Return '1' if you believe the question could be answered, inferred, attempted, or if you can even guess at the answer. Return '0' if you are certain the data is completely irrelevant to the question. If you return 0, please tell me the reason why.
`, jsonContent, userQuestion)},
	}

	response, err := s.OpenAIChat(&validationMessages)
	if err != nil {
		return false, fmt.Errorf("failed to validate user question with OpenAI: %v", err)
	}

	if response != "1" {
		log.Printf("User question validation response: %s", response)
	}

	return response == "1", nil
}

func (s Server) AnswerUserQuestionBasedJson(JsonContent string, userQuestion string) (string, error) {
	answerMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `You are an AI assistant that helps users answer questions by analyzing their JSON data. Your role is to analyze the Json given and answer the user's original question.`},
		{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(`Using the folowing JSON:
%s

Please answer the user's question in a kind and friendly way. The user asked the following question:
%s`, JsonContent, userQuestion)},
	}

	// Send the conversation to OpenAI and get the answer
	answer, err := s.OpenAIChat(&answerMessages)
	if err != nil {
		return "", fmt.Errorf("failed to get an answer from OpenAI: %v", err)
	}

	return strings.TrimSpace(answer), nil
}
