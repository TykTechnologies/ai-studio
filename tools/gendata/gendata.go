package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	daysToGenerate       = 30
	maxChatRecordsPerDay = 150
	maxToolCallsPerDay   = 100
)

var vendorModels = map[string][]string{
	string(models.ANTHROPIC): {"Claude-Sonnet", "Claude-Haiku", "Claude-Opus"},
	string(models.VERTEX):    {"llama-3b", "Claude-Sonnet", "Claude-Haiku"},
	string(models.OPENAI):    {"GPT-3.5-turbo", "GPT-4", "GPT-4o"},
}

var vendorCurrencies = map[string][]string{
	string(models.ANTHROPIC): {"USD"},
	string(models.VERTEX):    {"EUR", "USD"},
	string(models.OPENAI):    {"USD"},
}

func main() {
	dbConnStr := flag.String("db", "", "Database connection string")
	dbType := flag.String("type", "mysql", "Database type (mysql, postgres, sqlite)")
	flag.Parse()

	if *dbConnStr == "" {
		log.Fatal("Database connection string is required")
	}

	db, err := connectToDatabase(*dbType, *dbConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Drop existing tables
	err = dropTables(db)
	if err != nil {
		log.Fatalf("Failed to drop tables: %v", err)
	}

	err = db.AutoMigrate(&analytics.LLMChatRecord{}, &analytics.LLMChatLogEntry{}, &analytics.ToolCallRecord{})
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	generateTestData(db)
	fmt.Println("Test data generation completed.")
}

func connectToDatabase(dbType, connStr string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch dbType {
	case "mysql":
		dialector = mysql.Open(connStr)
	case "postgres":
		dialector = postgres.Open(connStr)
	case "sqlite":
		dialector = sqlite.Open(connStr)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	return gorm.Open(dialector, &gorm.Config{})
}

func dropTables(db *gorm.DB) error {
	err := db.Migrator().DropTable(&analytics.LLMChatRecord{}, &analytics.LLMChatLogEntry{}, &analytics.ToolCallRecord{})
	if err != nil {
		return fmt.Errorf("failed to drop tables: %v", err)
	}
	return nil
}

func generateTestData(db *gorm.DB) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -daysToGenerate)

	for day := 0; day < daysToGenerate; day++ {
		currentDate := startDate.AddDate(0, 0, day)
		chatRecordsToCreate := rand.Intn(maxChatRecordsPerDay) + 1
		toolCallsToCreate := rand.Intn(maxToolCallsPerDay) + 1

		for i := 0; i < chatRecordsToCreate; i++ {
			createChatRecord(db, currentDate)
			createChatLogEntry(db, currentDate)
		}

		for i := 0; i < toolCallsToCreate; i++ {
			createToolCallRecord(db, currentDate)
		}
	}
}

func createChatRecord(db *gorm.DB, date time.Time) {
	vendor := randomChatVendor()
	record := &analytics.LLMChatRecord{
		Name:           randomChatModelName(vendor),
		Vendor:         vendor,
		TotalTimeMS:    rand.Intn(10000) + 500,
		PromptTokens:   rand.Intn(200) + 50,
		ResponseTokens: rand.Intn(500) + 100,
		TotalTokens:    rand.Intn(700) + 150,
		TimeStamp:      randomTimeOnDate(date),
		UserID:         uint(rand.Intn(100) + 1),
		Choices:        rand.Intn(4) + 1,
		ToolCalls:      rand.Intn(3),
		ChatID:         fmt.Sprintf("chat_%d", rand.Int()),
		AppID:          uint(rand.Intn(10) + 1),
		Cost:           rand.Float64() * 0.5,
		Currency:       randomCurrency(vendor),
	}
	db.Create(record)
}

func createChatLogEntry(db *gorm.DB, date time.Time) {
	vendor := randomChatVendor()
	entry := &analytics.LLMChatLogEntry{
		Name:      randomChatModelName(vendor),
		Vendor:    vendor,
		TimeStamp: randomTimeOnDate(date),
		Prompt:    randomPrompt(),
		Response:  randomResponse(),
		Tokens:    rand.Intn(700) + 150,
		UserID:    uint(rand.Intn(100) + 1),
	}
	db.Create(entry)
}

func createToolCallRecord(db *gorm.DB, date time.Time) {
	record := &analytics.ToolCallRecord{
		ToolID:    uint(rand.Intn(20) + 1),
		Name:      randomToolName(),
		ExecTime:  rand.Intn(2000) + 50,
		TimeStamp: randomTimeOnDate(date),
	}
	db.Create(record)
}

func randomChatModelName(vendor string) string {
	models := vendorModels[vendor]
	return models[rand.Intn(len(models))]
}

func randomChatVendor() string {
	vendors := []string{string(models.OPENAI), string(models.ANTHROPIC), string(models.VERTEX)}
	return vendors[rand.Intn(len(vendors))]
}

func randomToolName() string {
	tools := []string{"Calculator", "WeatherAPI", "WikipediaSearch", "ImageGenerator", "TextTranslator"}
	return tools[rand.Intn(len(tools))]
}

func randomPrompt() string {
	prompts := []string{
		"What is the capital of France?",
		"Explain the theory of relativity",
		"Write a short story about a time traveler",
		"How does photosynthesis work?",
		"Describe the plot of Romeo and Juliet",
	}
	return prompts[rand.Intn(len(prompts))]
}

func randomResponse() string {
	responses := []string{
		"The capital of France is Paris.",
		"The theory of relativity, proposed by Albert Einstein, describes how...",
		"In the year 2150, Dr. Emily Chen activated her time machine...",
		"Photosynthesis is the process by which plants use sunlight to convert...",
		"Romeo and Juliet is a tragedy written by William Shakespeare about two young lovers...",
	}
	return responses[rand.Intn(len(responses))]
}

func randomTimeOnDate(date time.Time) time.Time {
	return date.Add(time.Duration(rand.Intn(24*60*60)) * time.Second)
}

func randomCurrency(vendor string) string {
	currencies := vendorCurrencies[vendor]
	return currencies[rand.Intn(len(currencies))]
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
