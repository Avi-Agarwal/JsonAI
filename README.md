
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

- **Endpoint**: `/json-ai/login`
- **Method**: `POST`
- **Request**:
  - `email`: User's email.
  - `pin`: User's PIN.
- **Response**:
  - `user`: Object containing user details such as user ID and token.

### 2. View Existing Chats

Upon login, users can see their ongoing or past chats and choose to continue from where they left off.

- **Endpoint**: `/json-ai/user/{userID}/chats`
- **Method**: `GET`
- **Response**:
  - A list of chats associated with the user.
  - Each chat includes:
    - `chatID`: Identifier for the chat.
    - `title`: The name of the JSON file used to start the chat.

### 3. Start a New Chat

Users can initiate a new chat by uploading a valid JSON file. The server saves the file to a temporary directory, uploads it to S3, and starts a new chat session. If the JSON is small enough (token estimate under 2000), it is cached in the database for faster access.

- **Endpoint**: `/json-ai/user/{userID}/upload-json`
- **Method**: `POST`
- **Request**:
  - **Path Parameter**:
    - `userID`: The user's ID.
  - **File Upload**:
    - The request should include a valid JSON file.
- **Response**:
  - `chatID`: Identifier for the new chat session.
  - `chat`: The newly created chat object, which includes:
    - `chatID`: Unique identifier for the chat.
    - `userID`: The user's ID associated with the chat.
    - `jsonName`: The name of the uploaded JSON file.
    - `messages`: A list containing the initial assistant message confirming successful upload.

- **Error Handling**:
  - If the user ID is not found in the database, a `400 Bad Request` error is returned.
  - If the uploaded file is not valid JSON, a `400 Bad Request` error is returned.
  - If there are any internal errors while saving the file or starting the chat, a `500 Internal Server Error` is returned.

### 4. Chat with JsonAI

Users can ask questions about the uploaded JSON file. The conversation is stored and can be retrieved later.

- **Endpoint**: `/json-ai/user/{userID}/chat/{chatID}`
- **Method**:
  - `PUT`: Send a new question to JsonAI.
  - **Request**:
    - `question`: The user's question about the uploaded JSON data.
    - `chatID`: The chat session ID.
  - **Response**:
    - `answer`: AI-generated answer to the user's question.
    - `chat`: Updated chat with a history of all previous exchanges.
  - **Error Handling**:
    - If the user's question cannot be answered using the JSON file, the response will indicate this: `“I cannot answer the query using the information from the file.”`

### 5. Retrieve a Specific Chat

Users can retrieve a specific chat session by using the chat ID.

- **Endpoint**: `/json-ai/user/{userID}/chat/{chatID}`
- **Method**: `GET`
- **Response**:
  - `chat`: The full chat history for the specified chat session.

---

## API Endpoints Summary

| Endpoint                              | Method | Description                                 |
|---------------------------------------|--------|---------------------------------------------|
| `/json-ai/login`                      | POST   | User login to receive tokens                |
| `/json-ai/user/{userID}/chats`        | GET    | Retrieve a list of the user's chat sessions |
| `/json-ai/user/{userID}/upload-json`  | POST   | Upload a JSON file to start a new chat      |
| `/json-ai/user/{userID}/chat/{chatID}`| GET    | Retrieve the current chat state             |
| `/json-ai/user/{userID}/chat/{chatID}`| PUT    | Send a message and get a response from JsonAI|

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

  #### On Ubuntu:
  ```bash
  sudo apt update
  sudo apt install postgresql postgresql-contrib
  ```

  #### On macOS:
  ```bash
  brew install postgresql
  ```

- Start PostgreSQL service:

  #### On Ubuntu:
  ```bash
  sudo service postgresql start
  ```

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
sudo service postgresql start   # Ubuntu
brew services start postgresql  # macOS
```

### 7. Start the server:

```bash
go run main.go
```