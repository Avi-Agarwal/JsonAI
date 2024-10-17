package db

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"time"
)

func getDatabaseDSN() (string, logger.Interface) {
	host := os.Getenv("JAI_DB_HOST")
	port := os.Getenv("JAI_DB_PORT")
	user := os.Getenv("JAI_DB_USER")
	password := os.Getenv("JAI_DB_PASSWORD")
	dbname := os.Getenv("JAI_DB_NAME")
	sslmode := os.Getenv("JAI_DB_SSL_MODE")
	timezone := os.Getenv("JAI_DB_TIMEZONE")
	isLogOn := os.Getenv("JAI_DB_LOG")

	logLevel := logger.Info
	if isLogOn != "true" {
		logLevel = logger.Error
	}

	dbLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold: time.Second, // Slow SQL threshold
			LogLevel:      logLevel,    // Log level (Silent, Error, Warn, Info)
			Colorful:      true,        // Disable color
		},
	)

	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		host, user, password, dbname, port, sslmode, timezone), dbLogger
}

func InitDB() *gorm.DB {
	dsn, dbLogger := getDatabaseDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: dbLogger})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	err = db.AutoMigrate(Tables...)
	if err != nil {
		log.Println("Error migrating database: ", err)
		return nil
	}

	return db
}
