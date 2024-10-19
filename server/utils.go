package server

import (
	"fmt"
	"strings"
)

func estimateTokenCount(content string) int {
	return len(strings.Split(content, " "))
}

func getJSONPreview(jsonContent string, previewLength int) string {
	if len(jsonContent) > previewLength {
		return jsonContent[:previewLength] + "..." // Add ellipsis to indicate truncation
	}
	return jsonContent
}

func cleanSQLGeneration(sqlQuery string) string {
	sqlQuery = strings.TrimSpace(sqlQuery)

	sqlQuery = strings.TrimPrefix(sqlQuery, "```sql")
	sqlQuery = strings.TrimSuffix(sqlQuery, "```")

	return strings.TrimSpace(sqlQuery)
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

func getBucketAndKeyFromS3URL(url string) (bucket, key string, err error) {
	if !strings.HasPrefix(url, "https://") {
		return "", "", fmt.Errorf("invalid S3 URL format")
	}

	urlParts := strings.Split(strings.TrimPrefix(url, "https://"), "/")

	// Handle cases where the URL might not have the correct structure
	if len(urlParts) < 2 {
		return "", "", fmt.Errorf("invalid S3 URL format")
	}

	hostParts := strings.Split(urlParts[0], ".")
	if len(hostParts) < 4 || hostParts[1] != "s3" {
		return "", "", fmt.Errorf("invalid S3 URL format")
	}
	bucket = hostParts[0]

	// The key is the rest of the URL
	key = strings.Join(urlParts[1:], "/")

	return bucket, key, nil
}
