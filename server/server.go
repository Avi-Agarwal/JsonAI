package server

import (
	"JsonAI/db"
	"JsonAI/proto"
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/lpernett/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Server struct {
	HTTPPort   string
	GRPCPort   string
	DB         *gorm.DB
	AWS        AwsConfig
	OpenApiKey string
	proto.UnimplementedJsonAIServiceServer
}

type AwsConfig struct {
	AccessKey  string
	SecretKey  string
	Region     string
	BucketName string
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func NewServer() *Server {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	httpPort := getEnv("JAI_SERVER_PORT", "1024")
	grpcPort := getEnv("JAI_GRPC_PORT", "1030")
	openAIKey := getEnv("JAI_OPENAI_KEY", "")

	awsConfig := AwsConfig{
		AccessKey:  getEnv("JAI_AWS_ACCESS_KEY", ""),
		SecretKey:  getEnv("JAI_AWS_SECRET_KEY", ""),
		Region:     getEnv("JAI_AWS_REGION", "us-east-1"),
		BucketName: getEnv("JAI_AWS_BUCKET", ""),
	}

	log.Println("Connecting to DB...")
	dbConn := db.InitDB()
	if dbConn == nil {
		log.Fatalln("Failed to connect to database")
	}

	return &Server{
		HTTPPort:   httpPort,
		GRPCPort:   grpcPort,
		OpenApiKey: openAIKey,
		DB:         dbConn,
		AWS:        awsConfig,
	}
}

func (s Server) setupGRPC() error {
	// Create a listener on TCP port
	tcpAddress := fmt.Sprintf(":%s", s.GRPCPort)
	lis, err := net.Listen("tcp", tcpAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on grpc port: %v", err)
	}

	// Create a gRPC server object
	g := grpc.NewServer()
	proto.RegisterJsonAIServiceServer(g, s)

	log.Printf("Serving gRPC on 0.0.0.0:%s", s.GRPCPort)
	go func() {
		if err := g.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	return nil
}

func (s Server) setupHTTP() error {
	// Create a client connection to the gRPC server
	grpcTarget := fmt.Sprintf("0.0.0.0:%s", s.GRPCPort)
	conn, err := grpc.NewClient(
		grpcTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to dial gRPC server: %v", err)
	}

	// Create a new gRPC gateway mux
	gwmux := runtime.NewServeMux()

	// Register the gateway handler with the mux
	err = proto.RegisterJsonAIServiceHandler(context.Background(), gwmux, conn)
	if err != nil {
		return fmt.Errorf("failed to register gateway: %v", err)
	}

	// Default HTTP port
	port := os.Getenv("PORT")
	if port == "" {
		port = "1024"
	}

	// Create a new HTTP router
	r := mux.NewRouter()
	r.HandleFunc("/json-ai/user/{userID}/upload-json", s.handleJsonUpload).Methods("POST")

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			return
		}
	}).Methods("GET")

	r.PathPrefix("/").Handler(gwmux)

	log.Printf("Http server starting on 0.0.0.0:%s", port)
	httpAddress := fmt.Sprintf(":%s", port)
	gwServer := &http.Server{
		Addr:    httpAddress,
		Handler: r,
	}

	go func() {
		if err := gwServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start HTTP Http: %v", err)
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt)

	<-shutdownSignal
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := gwServer.Shutdown(ctx); err != nil {
		log.Fatalf("Http server forced to shutdown: %v", err)
	}

	log.Println("Server shut down")
	return nil
}

func (s Server) Serve() error {
	if err := s.setupGRPC(); err != nil {
		return err
	}

	if err := s.setupHTTP(); err != nil {
		return err
	}

	return nil
}
