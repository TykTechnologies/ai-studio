package main

import (
	"log"

	"github.com/TykTechnologies/midsommar/v2/api"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	// Open a connection to the SQLite database
	// If the file doesn't exist, it will be created
	db, err := gorm.Open(sqlite.Open("midsommar.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto Migrate the schemas
	err = models.InitModels(db)
	if err != nil {
		log.Fatalf("Failed to initialize models: %v", err)
	}

	// Create a new service instance
	service := services.NewService(db)

	// Create a new API instance
	api := api.NewAPI(service, true) // true to disable CORS for development

	// Run the API
	if err := api.Run(":8080"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
