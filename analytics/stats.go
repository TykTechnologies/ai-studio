package analytics

import (
	"strconv"
	"time"

	"gorm.io/gorm"
)

type ChartData struct {
	Labels []string  `json:"labels"`
	Data   []float64 `json:"data"`
}

// GetChatRecordsPerDay returns the total number of chat records per day for a given time period
func GetChatRecordsPerDay(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		Date  string
		Count int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = result.Date // Use the date string directly
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

func GetToolCallsPerDay(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		Date  string
		Count int64
	}

	err := db.Model(&ToolCallRecord{}).
		Select("DATE(time_stamp) as date, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = result.Date // Use the date string directly
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

// GetChatRecordsPerUser returns the total number of chat records per user for a given time period
func GetChatRecordsPerUser(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		UserID uint
		Count  int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("user_id, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("user_id").
		Order("count DESC").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = getUserName(db, result.UserID) // You'll need to implement this function
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

// Helper function to get user name (you'll need to implement this based on your user model)
func getUserName(db *gorm.DB, userID uint) string {
	// Implement this function to retrieve the user's name or username
	// based on the userID. For example:
	// var user User
	// db.First(&user, userID)
	// return user.Name
	strUserID := strconv.Itoa(int(userID))
	return "User " + strUserID // Placeholder implementation
}
