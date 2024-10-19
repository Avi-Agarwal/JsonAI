package server

import (
	"JsonAI/db"
	"JsonAI/proto"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/marcboeker/go-duckdb"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"log"
	"time"
)

const (
	invalidQuestionResponse = "I cannot answer the query using the information from the file"
	maxTokenLimit           = 25000
	maxResultTokenLimit     = 18000
)

func (s Server) GetChat(ctx context.Context, in *proto.GetChat_Request) (*proto.GetChat_Response, error) {
	if in.UserID == "" {
		return nil, status.Error(codes.InvalidArgument, "UserID is required")
	}

	if in.ChatID == "" {
		return nil, status.Error(codes.InvalidArgument, "ChatID is required")
	}

	_, err := db.GetUserByID(s.DB, in.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "User not found")
		} else {
			log.Printf("Failed to retrieve user: %s", err)
			return nil, status.Error(codes.Internal, "Failed to retrieve user")
		}
	}

	jChat, messages, err := db.GetChatByID(s.DB, in.ChatID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "Chat not found")
		} else {
			log.Printf("Failed to retrieve chat: %s", err)
			return nil, status.Error(codes.Internal, "Failed to retrieve chat")
		}
	}

	protoMessages := make([]*proto.Message, 0, len(messages))
	for _, message := range messages {
		protoMessages = append(protoMessages, &proto.Message{
			Role:      message.Role,
			Message:   message.Message,
			CreatedAt: message.CreatedAt.Format(time.RFC3339),
		})
	}

	return &proto.GetChat_Response{
		Chat: &proto.Chat{
			ChatID:       jChat.UUID.ID,
			UserID:       jChat.UserID,
			JsonName:     jChat.JSON,
			MessageCount: int32(len(protoMessages)),
			Messages:     protoMessages,
		},
	}, nil
}

func (s Server) ListChats(ctx context.Context, in *proto.ListChats_Request) (*proto.ListChats_Response, error) {
	if in.UserID == "" {
		return nil, status.Error(codes.InvalidArgument, "UserID is required")
	}

	_, err := db.GetUserByID(s.DB, in.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "User not found")
		} else {
			log.Printf("Failed to retrieve user: %s", err)
			return nil, status.Error(codes.Internal, "Failed to retrieve user")
		}
	}

	chats, err := db.GetChatsByUserID(s.DB, in.UserID)
	if err != nil {
		log.Printf("Failed to retrieve chats: %s", err)
		return nil, status.Error(codes.Internal, "Failed to retrieve chats")
	}

	protoChats := make([]*proto.Chat, 0, len(chats))
	for _, chat := range chats {
		messageCnt, err := db.GetChatMessageCount(s.DB, chat.UUID.ID)
		if err != nil {
			log.Printf("Failed to retrieve message count: %s", err)
			return nil, status.Error(codes.Internal, "Failed to retrieve message count")
		}
		protoChats = append(protoChats, &proto.Chat{
			ChatID:       chat.UUID.ID,
			UserID:       chat.UserID,
			JsonName:     chat.JSON,
			MessageCount: int32(messageCnt),
		})
	}

	return &proto.ListChats_Response{
		Chats: protoChats,
	}, nil
}

func (s Server) AskJsonAI(ctx context.Context, in *proto.AskJsonAI_Request) (*proto.AskJsonAI_Response, error) {
	if in.UserID == "" {
		return nil, status.Error(codes.InvalidArgument, "UserID is required")
	}

	if in.ChatID == "" {
		return nil, status.Error(codes.InvalidArgument, "ChatID is required")
	}

	if in.Question == "" {
		return nil, status.Error(codes.InvalidArgument, "Please ask a question")
	}

	jaiChat, _, err := db.GetChatByID(s.DB, in.ChatID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "Chat not found")
		} else {
			log.Printf("Failed to retrieve chat: %s", err)
			return nil, status.Error(codes.Internal, "Failed to retrieve chat")
		}
	}

	if jaiChat.FileTokenEstimate < 2000 {
		return s.handleSmallJson(ctx, in.Question, jaiChat)
	}
	return s.handleLargeJson(ctx, in.Question, jaiChat)
}

func (s Server) handleLargeJson(ctx context.Context, userQuestion string, jChat *db.JaiChat) (*proto.AskJsonAI_Response, error) {
	s3FileLocation := jChat.FileLocation
	bucket, key, err := getBucketAndKeyFromS3URL(s3FileLocation)
	if err != nil {
		log.Printf("Failed to parse S3 URL: %s", err)
		return nil, status.Error(codes.Internal, "Failed to parse S3 URL")
	}

	// Download the file from S3
	jsonContent, err := s.DownloadFileFromS3(bucket, key)
	if err != nil {
		log.Printf("Failed to download the file from S3: %s", err)
		return nil, status.Error(codes.Internal, "Failed to download the file")
	}

	jsonPreview := getJSONPreview(jsonContent, 5000)

	var jsonData interface{}
	err = json.Unmarshal([]byte(jsonContent), &jsonData)
	if err != nil {
		log.Printf("Failed to unmarshal JSON: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	duckDB, err := sql.Open("duckdb", "")
	if err != nil {
		log.Printf("Failed to open DuckDB: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}
	defer func(duckDB *sql.DB) {
		err := duckDB.Close()
		if err != nil {
			log.Printf("Failed to close DuckDB: %s", err)
		}
	}(duckDB)

	// Create a new table in DuckDB
	err = createTableFromJSON(duckDB, jsonData)
	if err != nil {
		log.Printf("Failed to create table from JSON: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	err = insertDataIntoDuckDB(duckDB, jsonData)
	if err != nil {
		log.Printf("Failed to insert data into DuckDB: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}
	tableName := "json_data"

	schema, err := getTableSchema(duckDB, tableName)
	if err != nil {
		log.Printf("Failed to get schema for main table: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	uniqueFields, err := extractUniqueFieldsFromJSONColumns(duckDB, tableName)
	if err != nil {
		log.Printf("Error extracting JSON fields: %v", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	totalSchema := "Table Schema:\n" + schema
	if len(uniqueFields) > 0 {
		totalSchema = totalSchema + "\n\nExpanded Fields from JSON Columns:\n" + uniqueFields
	}

	// Output the schema
	fmt.Println("Total schema:")
	fmt.Println(totalSchema)

	// Insert user message
	userMessage := &db.ChatMessages{
		JaiChatID: jChat.UUID.ID,
		Role:      openai.ChatMessageRoleUser,
		Message:   userQuestion,
		Model: gorm.Model{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	err = db.AddChatMessage(s.DB, userMessage)
	if err != nil {
		log.Printf("Failed to add user message: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	isValidQuestion, err := s.ValidateUserQuestion(userQuestion, totalSchema, jsonPreview, jChat.JSON)
	if err != nil {
		log.Printf("Failed to validate user question: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	// If the user question is not valid, response with the invalid question message
	if !isValidQuestion {
		systemMessage := &db.ChatMessages{
			JaiChatID: jChat.UUID.ID,
			Role:      openai.ChatMessageRoleAssistant,
			Message:   invalidQuestionResponse,
			Model: gorm.Model{
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}
		err = db.AddChatMessage(s.DB, systemMessage)
		if err != nil {
			log.Printf("Failed to add system message: %s", err)
			return nil, status.Error(codes.Internal, "Internal server error")
		}

		getChatResp, err := s.GetChat(ctx, &proto.GetChat_Request{
			UserID: jChat.UserID,
			ChatID: jChat.UUID.ID,
		})
		if err != nil {
			log.Printf("Failed to get chat: %s", err)
			return nil, status.Error(codes.Internal, "Internal server error")
		}

		return &proto.AskJsonAI_Response{
			Answer: invalidQuestionResponse,
			Chat:   getChatResp.Chat,
		}, nil
	}

	results, sqlQuery, err := s.ConvertUserQuestionToSQLAndRetrieveQueryResults(duckDB, userQuestion, tableName, totalSchema, jsonPreview)
	if err != nil {
		log.Printf("Failed to convert user question to SQL: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	resultsString := fmt.Sprintf("Query Run: %s\nQuery Results: %s\n", sqlQuery, results)
	result1EstimatedTokens := estimateTokenCount(resultsString)

	if result1EstimatedTokens >= maxResultTokenLimit {
		log.Printf("Skipping second query because the first result is already too large. Estimated tokens: %d", result1EstimatedTokens)
	} else {
		// Run the second query and append its result
		results, sqlQuery, err = s.RetrieveRelevantInformation(duckDB, userQuestion, tableName, totalSchema, jsonPreview)
		if err != nil {
			log.Fatalf("Error running SQL query: %v", err)
		}

		// Include the query run information in the token count
		result2String := fmt.Sprintf("\nQuery Run: %s\nQuery Results: %s\n", sqlQuery, results)
		result2EstimatedTokens := estimateTokenCount(result2String)

		// If the combined token usage is under the limit, append the second result
		if result1EstimatedTokens+result2EstimatedTokens <= maxTokenLimit {
			resultsString = resultsString + result2String
		} else {
			log.Printf("Skipping appending second result as it exceeds the token limit. Total tokens: %d", result1EstimatedTokens+result2EstimatedTokens)
		}
	}

	finalAnswer, err := s.AnswerUserQuestionBasedOnSQlResults(resultsString, userQuestion)
	if err != nil {
		log.Printf("Failed to answer user question: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	systemMessage := &db.ChatMessages{
		JaiChatID: jChat.UUID.ID,
		Role:      openai.ChatMessageRoleAssistant,
		Message:   finalAnswer,
		Model: gorm.Model{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	err = db.AddChatMessage(s.DB, systemMessage)
	if err != nil {
		log.Printf("Failed to add system message: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	getChatResp, err := s.GetChat(ctx, &proto.GetChat_Request{
		UserID: jChat.UserID,
		ChatID: jChat.UUID.ID,
	})
	if err != nil {
		log.Printf("Failed to get chat: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	return &proto.AskJsonAI_Response{
		Answer: finalAnswer,
		Chat:   getChatResp.Chat,
	}, nil
}

func (s Server) handleSmallJson(ctx context.Context, userQuestion string, jChat *db.JaiChat) (*proto.AskJsonAI_Response, error) {
	// Retrieve the JSON content
	var jsonContent string
	jCache, err := db.GetJsonFromCache(s.DB, jChat.UUID.ID)
	if err == nil {
		jsonContent = jCache.JSONContent
		err = db.UpdateLastAccess(s.DB, jCache.UUID.ID)
		if err != nil {
			log.Printf("Failed to update last access: %s", err)
		}
	} else {
		s3FileLocation := jChat.FileLocation
		bucket, key, err := getBucketAndKeyFromS3URL(s3FileLocation)
		if err != nil {
			log.Printf("Failed to parse S3 URL: %s", err)
			return nil, status.Error(codes.Internal, "Failed to parse S3 URL")
		}

		// Download the file from S3
		jsonContent, err = s.DownloadFileFromS3(bucket, key)
		if err != nil {
			log.Printf("Failed to download the file from S3: %s", err)
			return nil, status.Error(codes.Internal, "Failed to download the file")
		}

		// Insert the JSON content into the cache
		go func() {
			err := db.InsertJSONCache(s.DB, jChat.UUID.ID, jsonContent)
			if err != nil {
				log.Printf("Failed to insert JSON cache: %s", err)
			}
		}()
	}

	// Insert user message
	userMessage := &db.ChatMessages{
		JaiChatID: jChat.UUID.ID,
		Role:      openai.ChatMessageRoleUser,
		Message:   userQuestion,
		Model: gorm.Model{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	err = db.AddChatMessage(s.DB, userMessage)
	if err != nil {
		log.Printf("Failed to add user message: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	isValidQuestion, err := s.ValidateUserQuestionBasedOnJson(userQuestion, jsonContent)
	if err != nil {
		log.Printf("Failed to validate user question: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	// If the user question is not valid, response with the invalid question message
	if !isValidQuestion {
		systemMessage := &db.ChatMessages{
			JaiChatID: jChat.UUID.ID,
			Role:      openai.ChatMessageRoleAssistant,
			Message:   invalidQuestionResponse,
			Model: gorm.Model{
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}
		err = db.AddChatMessage(s.DB, systemMessage)
		if err != nil {
			log.Printf("Failed to add system message: %s", err)
			return nil, status.Error(codes.Internal, "Internal server error")
		}

		getChatResp, err := s.GetChat(ctx, &proto.GetChat_Request{
			UserID: jChat.UserID,
			ChatID: jChat.UUID.ID,
		})
		if err != nil {
			log.Printf("Failed to get chat: %s", err)
			return nil, status.Error(codes.Internal, "Internal server error")
		}

		return &proto.AskJsonAI_Response{
			Answer: invalidQuestionResponse,
			Chat:   getChatResp.Chat,
		}, nil
	}

	answer, err := s.AnswerUserQuestionBasedJson(jsonContent, userQuestion)
	if err != nil {
		log.Printf("Failed to answer user question: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	systemMessage := &db.ChatMessages{
		JaiChatID: jChat.UUID.ID,
		Role:      openai.ChatMessageRoleAssistant,
		Message:   answer,
		Model: gorm.Model{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	err = db.AddChatMessage(s.DB, systemMessage)
	if err != nil {
		log.Printf("Failed to add system message: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	getChatResp, err := s.GetChat(ctx, &proto.GetChat_Request{
		UserID: jChat.UserID,
		ChatID: jChat.UUID.ID,
	})
	if err != nil {
		log.Printf("Failed to get chat: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	return &proto.AskJsonAI_Response{
		Answer: invalidQuestionResponse,
		Chat:   getChatResp.Chat,
	}, nil
}
