# JsonAI

JsonAI is a server-side application that allows users to upload JSON files and interact with an AI to ask questions about the content of the uploaded file. The AI processes the JSON data and answers user queries based on the information available in the file.

## Features

- **User Authentication**: Simple login system with username and password/PIN.
- **JSON Upload**: Users can upload JSON files to start a new chat.
- **Interactive Q&A**: Users can ask questions about their uploaded JSON file, and the AI provides answers based on the content.
- **Chat History**: Users can view and resume previous chats.
- **Error Handling**: Robust system to manage invalid JSON uploads and unsupported queries.

## Application Workflow

### 1. User Login

Users authenticate by providing a username and a password or PIN.

- **Endpoint**: `/json-ai/login`
- **Response**:
    - `user-id`: Unique identifier for the user.
    - `jwt token`: Token for session management and authorization.
    - `refresh token`: Token for renewing the session.

### 2. View Existing Chats

Upon login, users can see their ongoing or past chats and choose to continue from where they left off.

- **Endpoint**: `/json-ai/chats`
- **Response**:
    - A list of chats associated with the user.
    - Each chat includes:
        - `chat-id`: Identifier for the chat.
        - `title`: The name of the JSON file used to start the chat.

### 3. Start a New Chat

Users can initiate a new chat by uploading a valid JSON file. The server saves the file and starts a new session.

- **Endpoint**: `/json-ai/upload`
- **Request**:
    - JSON file to be uploaded.
- **Response**:
    - `chat-id`: Identifier for the new chat session.
    - `title`: Name of the uploaded JSON file.
- **Error Handling**:
    - Only valid JSON files are accepted.
    - Files exceeding the size limit are rejected with an appropriate error message.

### 4. Chat with JsonAI

Users can ask questions about the uploaded JSON file. The conversation is stored and can be retrieved later.

- **Endpoint**: `/json-ui/chat/{chatID}`
- **Methods**:
    - `GET`: Retrieve the current chat history between the user and JsonAI.
    - `PUT`: Send a new message to JsonAI and receive a response.
- **Caching**:
    - The server temporarily stores the JSON file associated with the chat in memory for quicker response times.
    - If the JSON is not in memory, or if it's associated with a different chat, the server retrieves it from the bucket.
- **Error Handling**:
    - If a query cannot be answered based on the JSON file, the response will be:
        - `“I cannot answer the query using the information from the file.”`
    - User token limits are enforced to prevent system abuse.

### 5. Resume a Previous Chat

Users can select and resume a previous chat session from their chat history.

- **Flow**:
    - Use the chat ID from `/json-ai/chats` to continue interacting with the same JSON file.

---

## API Endpoints Summary

| Endpoint                        | Method | Description                              |
|----------------------------------|--------|------------------------------------------|
| `/json-ai/login`                 | POST   | User login to receive tokens             |
| `/json-ai/chats`                 | GET    | Retrieve a list of the user's chat sessions |
| `/json-ai/upload`                | POST   | Upload a JSON file to start a new chat   |
| `/json-ui/chat/{chatID}`         | GET    | Retrieve the current chat state          |
| `/json-ui/chat/{chatID}`         | PUT    | Send a message and get a response from JsonAI |

---

## Installation

1. Clone the repository:

```bash
git clone https://github.com/your-username/JsonAI.git
```

2. Navigate to the project directory:

```bash
cd JsonAI
```

3. Install dependencies:

```bash
go mod tidy
```

4. Setup environment variables by creating a `.env` file:

```bash
JAI_SERVER_PORT=1024
JAI_GRPC_PORT=1030
JAI_OPENAI_KEY=your-openai-key
JAI_AWS_ACCESS_KEY=your-aws-access-key
JAI_AWS_SECRET_KEY=your-aws-secret-key
JAI_AWS_REGION=us-east-1
JAI_AWS_BUCKET=your-bucket-name
```

5. Start the server:

```bash
go run main.go
```

---

## Future Improvements

- **Enhanced Error Messaging**: Provide more detailed error feedback to users.
- 

---

This version of the README provides a more professional structure, clear sectioning, and enhanced readability. It should now be much easier for contributors and users to understand your project and how to interact with it. Let me know if you'd like to add or change anything!