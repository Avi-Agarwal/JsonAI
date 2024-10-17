package server

import (
	"encoding/json"
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

	if err := validateJSON(fileBytes); err != nil {
		logErrorAndRespond(w, "Uploaded file is not a valid JSON", err, http.StatusBadRequest)
		return
	}

	filePath := filepath.Join("./tmp", handler.Filename)
	if err := saveFileToDisk(filePath, fileBytes); err != nil {
		logErrorAndRespond(w, "Failed to save the file", err, http.StatusInternalServerError)
		return
	}

	log.Printf("File uploaded successfully: %s", filePath)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("File uploaded successfully"))
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

func validateJSON(data []byte) error {
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return fmt.Errorf("invalid JSON file: %v", err)
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
