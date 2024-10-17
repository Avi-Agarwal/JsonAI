package server

import (
	"JsonAI/db"
	"JsonAI/proto"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

const (
	MaxFileSize = 10 // 10 MB
)

func (s Server) handleJsonUpload(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["userID"]
	if userID == "" {
		http.Error(w, "userID is required", http.StatusBadRequest)
		return
	}

	// Parse and validate the file upload
	file, handler, err := parseAndValidateFileUpload(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer closeFile(file)

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		logErrorAndRespond(w, "Error reading file", err, http.StatusInternalServerError)
		return
	}

	if err := validateJSONFile(handler.Filename); err != nil {
		logErrorAndRespond(w, "Uploaded file is not a JSON", err, http.StatusBadRequest)
		return
	}

	filePath := filepath.Join("./tmp", handler.Filename)
	if err := saveFileToDisk(filePath, fileBytes); err != nil {
		logErrorAndRespond(w, "Failed to save the file", err, http.StatusInternalServerError)
		return
	}
	log.Printf("File uploaded to tmp folder successfully: %s", filePath)

	// Upload the file to S3 and remove from tmp folder
	s3Location, err := s.UploadToS3(filePath, handler.Filename)
	if err != nil {
		log.Printf("Failed uploaded file to s3: %s", filePath)
		logErrorAndRespond(w, "Failed to upload file", err, http.StatusInternalServerError)
		return
	}

	jChat, err := db.StartChat(s.DB, userID, handler.Filename, s3Location)
	if err != nil {
		log.Printf("Failed to start chat: %v", err)
		logErrorAndRespond(w, "Failed to start chat", err, http.StatusInternalServerError)
		return
	}

	chatProto := &proto.Chat{
		ID:           jChat.UUID.ID,
		UserID:       jChat.UserID,
		JsonName:     jChat.JSON,
		MessageCount: 0,
		Messages:     nil,
	}

	// Marshal the proto message to JSON and return it to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(chatProto); err != nil {
		logErrorAndRespond(w, "Failed to encode chat object", err, http.StatusInternalServerError)
	}
}

func parseAndValidateFileUpload(r *http.Request) (multipart.File, *multipart.FileHeader, error) {
	const maxFileSize = MaxFileSize << 20
	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse form: %v", err)
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		return nil, nil, fmt.Errorf("error retrieving the file: %v", err)
	}

	if handler.Size > maxFileSize {
		return nil, nil, fmt.Errorf("file size exceeds the limit of %d MB", MaxFileSize)
	}

	return file, handler, nil
}

func validateJSONFormat(data []byte) error {
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return fmt.Errorf("invalid JSON file: %v", err)
	}
	return nil
}

func validateJSONFile(fileName string) error {
	if filepath.Ext(fileName) != ".json" {
		return errors.New("file is not a JSON file")
	}
	return nil
}

func saveFileToDisk(filePath string, data []byte) error {
	dst, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer closeFile(dst)

	_, err = dst.Write(data)
	if err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}
	return nil
}

func logErrorAndRespond(w http.ResponseWriter, message string, err error, statusCode int) {
	log.Printf("%s: %v", message, err)
	http.Error(w, fmt.Sprintf("%s: %v", message, err), statusCode)
}

func closeFile(f io.Closer) {
	err := f.Close()
	if err != nil {
		log.Printf("Error closing file: %v", err)
	}
}
