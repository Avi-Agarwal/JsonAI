
# JsonAI

JsonAI is a server-side application that allows users to upload JSON files and interact with an AI to ask questions about the content of the uploaded file. The AI processes the JSON data and answers user queries based on the information available in the file.

## Features

- **User Authentication**: Simple login system with email and PIN.
- **JSON Upload**: Users can upload JSON files to start a new chat.
- **Interactive Q&A**: Users can ask questions about their uploaded JSON file, and the AI provides answers based on the content.
- **Chat History**: Users can view and resume previous chats.

## Application Workflow

### 1. User Login

Users authenticate by providing an email and PIN.
Simple login system with email and PIN for basic user authentication.

- **Endpoint**: `/json-ai/login`
- **Method**: `POST`
- **Request**:
  - `email`: User's email.
  - `pin`: User's PIN.
- **Response**:
  - `user`: Object containing user details such as user ID and token.
  - `token`: JWT and refresh tokens for authentication. (Coming soon)

### 2. View Existing Chats

Upon login, users can see their ongoing or past chats and choose to continue from where they left off.
Client would show a list of chats on the user's dashboard, and when the user clicks on a chat, the client would the call GetChat with the ChatID.

- **Endpoint**: `/json-ai/user/{userID}/chats`
- **Method**: `GET`
- **Response**:
  - A list of chats associated with the user.
  - Each chat return includes:
    - `chatID`: Identifier for the chat.
    - `title`: The name of the JSON file used to start the chat.
    - `messageCount`: The number of messages exchanged in the chat.

### 3. Start a New Chat

Users can initiate a new chat session by uploading a valid JSON file. The server will store this file temporarily, upload it to S3, and create a new chat session. If the JSON is small enough (token estimate under 2000), it is cached in the database for faster access. The following steps explain how to correctly format the request and how the API processes the uploaded file.


- **Endpoint**: `/json-ai/user/{userID}/upload-json`
- **Method**: `POST`
- **Request**:
  - **Path Parameter**:
    - `userID`: The ID of the user starting the chat. It should be passed as part of the URL.
#### **File Upload**:
- **Form Data**:
  - The `file` parameter should be the JSON file to upload.
  - The file is uploaded using the `@` symbol in the `curl` command, which instructs `curl` to read the content of the file on the local system and send it to the server. For example, if your file is named `test.json`, you would pass it as `file=@test.json` in the `-F` option.

#### **Example cURL Request**:

```bash
curl -X POST http://localhost:1024/json-ai/user/{userID}/upload-json \
     -F "file=@test.json"
```

In this example:
- The `-F "file=@test.json"` flag tells `curl` to:
  - Use `-F` to specify that this is form data.
  - Attach the contents of `test.json` to the `file` form field.
  - The `@` symbol indicates that `curl` should read the file from the local filesystem and send its contents to the server.

- **Response**:
  - `chatID`: Identifier for the new chat session.
  - `chat`: The newly created chat object, which includes:
    - `chatID`: Unique identifier for the chat.
    - `userID`: The user's ID associated with the chat.
    - `jsonName`: The name of the uploaded JSON file.
    - `messages`: A list containing the initial assistant message confirming successful upload.

#### **Error Handling**:
- **400 Bad Request**: Returned if:
  - The user ID is not found in the database.
  - The uploaded file is not valid JSON.
- **500 Internal Server Error**: Returned if:
  - There is an internal error during file saving or chat session creation.


### 4. Chat with JSONAI

Once users have uploaded a JSON file, they can ask questions about the data. The system processes the question by analyzing the uploaded JSON, executing queries, and retrieving relevant results to answer the question. The entire chat, including the questions and answers, is stored and can be retrieved later for future reference.

#### **Endpoint**: `/json-ai/user/{userID}/chat/{chatID}`
- **Method**: `PUT` (Send a new question to JSONAI)

#### **Request**:
- **Path Parameters**:
  - `userID`: The ID of the user starting the chat.
  - `chatID`: The ID of the chat session (created during the JSON upload).
- **Body**:
  - `question`: The question the user wants to ask based on the uploaded JSON.

#### **Response**:
- `answer`: The AI-generated answer to the user's question.
- `chat`: The updated chat object containing the full history of all previous exchanges, including the original question, AI responses, and any system messages.

#### **Behavior Based on JSON File Size**:
- **Small JSON (under 2000 tokens)**:
  - If the uploaded JSON file is small enough, it will be cached in the database. When the user asks a question, the server retrieves the cached file and generates a response without having to download the file again from S3.
  - This approach speeds up responses for frequently asked questions or when the same file is referenced multiple times.

- **Large JSON (2000 tokens or more)**:
  - If the uploaded JSON file is large, it is not cached directly. Instead, the system retrieves the file from S3, parses it, and stores it temporarily for the duration of the chat. The server processes the file by loading the data into a DuckDB database to handle complex queries efficiently.

#### **Error Handling**:
- **Invalid Questions**: If the user's question cannot be answered using the JSON data (i.e., the question is unrelated to the data or does not match any relevant fields), the system will return an appropriate error message:
  - `"I cannot answer the query using the information from the file."`
- **Missing or Invalid Data**:
  - If the provided `userID` or `chatID` is invalid, or if the uploaded JSON file cannot be found, the system returns a `400 Bad Request` or `404 Not Found` error as appropriate.
  - If there is a problem with the internal processing (e.g., parsing or handling the file), a `500 Internal Server Error` is returned.

#### Example cURL Request:
```bash
curl -X PUT http://localhost:1024/json-ai/user/9e81a2d0-1574-43f1-a3b6-c5454d482d98/chat/12ab34cd56ef \
     -H "Content-Type: application/json" \
     -d '{"question": "How many entries are in the file?"}'
```

### 5. Retrieve a Specific Chat

Users can retrieve the full chat history of a specific chat session using the `chatID`. This is useful for reviewing past conversations and interactions with JSONAI. The response will include all messages exchanged during the session, including questions, answers, and system messages.

#### **Endpoint**: `/json-ai/user/{userID}/chat/{chatID}`
- **Method**: `GET`

#### **Path Parameters**:
- `userID`: The unique ID of the user.
- `chatID`: The ID of the chat session that the user wants to retrieve.

#### **Response**:
- **chat**: An object containing the full chat history for the specified session, including:
  - `chatID`: The unique identifier for the chat session.
  - `userID`: The ID of the user associated with the chat session.
  - `jsonName`: The name of the uploaded JSON file associated with the chat.
  - `messageCount`: The total number of messages exchanged during the chat session.
  - `messages`: An array of message objects, where each object includes:
    - `role`: The role of the message sender (`user` or `assistant`).
    - `message`: The content of the message.
    - `createdAt`: The timestamp indicating when the message was sent, in RFC3339 format.

#### Example cURL Request:

```bash
curl -X GET http://localhost:1024/json-ai/user/9e81a2d0-1574-43f1-a3b6-c5454d482d98/chat/12ab34cd56ef
```

#### Error Handling:
- **400 Bad Request**: Returned if the `userID` or `chatID` is missing or invalid.
- **404 Not Found**: Returned if the user or the chat session does not exist.
- **500 Internal Server Error**: Returned if there are any issues with retrieving the chat from the database.

---

## API Endpoints Summary

| Endpoint                              | Method | Description                                                              |
|---------------------------------------|--------|--------------------------------------------------------------------------|
| `/json-ai/login`                      | POST   | User login to receive authentication tokens.                             |
| `/json-ai/user/{userID}/chats`        | GET    | Retrieve a list of the user's previous chat sessions.                    |
| `/json-ai/user/{userID}/upload-json`  | POST   | Upload a JSON file to start a new chat. The file is saved and processed. |
| `/json-ai/user/{userID}/chat/{chatID}`| GET    | Retrieve the full chat history for a specific session.                   |
| `/json-ai/user/{userID}/chat/{chatID}`| PUT    | Ask a new question in an existing chat session and receive a response.   |

---

## Installation

### 1. Clone the repository:

```bash
git clone https://github.com/your-username/JsonAI.git
```

### 2. Navigate to the project directory:

```bash
cd JsonAI
```

### 3. Install dependencies:

```bash
go mod tidy
```

### 4. Setup PostgreSQL:

- Ensure PostgreSQL is installed and running on your machine. You can install PostgreSQL with:

  #### On macOS:
  ```bash
  brew install postgresql
  ```

- Start PostgreSQL service:

  #### On macOS:
  ```bash
  brew services start postgresql
  ```

- Set up a new database and user:

  ```bash
  sudo -u postgres psql
  CREATE DATABASE json_ai_db;
  CREATE USER json_ai_user WITH ENCRYPTED PASSWORD 'your-password';
  GRANT ALL PRIVILEGES ON DATABASE json_ai_db TO json_ai_user;
  ```

### 5. Setup environment variables:

Create a `.env` file with the following content:

```bash
JAI_SERVER_PORT=1024
JAI_GRPC_PORT=1030
JAI_OPENAI_KEY=your-openai-key
JAI_AWS_ACCESS_KEY=your-aws-access-key
JAI_AWS_SECRET_KEY=your-aws-secret-key
JAI_AWS_REGION=us-east-1
JAI_AWS_BUCKET=your-bucket-name
DB_HOST=localhost
DB_PORT=5432
DB_USER=json_ai_user
DB_PASSWORD=your-password
DB_NAME=json_ai_db
```

### 6. Start the PostgreSQL server:

Make sure your PostgreSQL server is running:

```bash
brew services start postgresql  # macOS
```

### 7. Start the server:

```bash
go run main.go
```

### 8. Demo:

#### **Basic user flow**:



https://github.com/user-attachments/assets/00cb2882-9808-461d-af1f-b2de411bd2f2



