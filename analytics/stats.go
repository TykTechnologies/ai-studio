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

// GetCostAnalysis returns the total cost per day for each currency
func GetCostAnalysis(db *gorm.DB, startDate, endDate time.Time) (map[string]*ChartData, error) {
	var results []struct {
		Date     string
		Currency string
		Cost     float64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, currency, SUM(cost) as cost").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("DATE(time_stamp), currency").
		Order("date, currency").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartDataMap := make(map[string]*ChartData)

	for _, result := range results {
		if _, exists := chartDataMap[result.Currency]; !exists {
			chartDataMap[result.Currency] = &ChartData{
				Labels: []string{},
				Data:   []float64{},
			}
		}
		chartDataMap[result.Currency].Labels = append(chartDataMap[result.Currency].Labels, result.Date)
		chartDataMap[result.Currency].Data = append(chartDataMap[result.Currency].Data, result.Cost)
	}

	return chartDataMap, nil
}

// GetMostUsedLLMModels returns the usage count for each LLM model
func GetMostUsedLLMModels(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		Name  string
		Count int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("name, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("name").
		Order("count DESC").
		Limit(10). // Limit to top 10 models
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = result.Name
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

// GetToolUsageStatistics returns the usage count for each tool
func GetToolUsageStatistics(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		Name  string
		Count int64
	}

	err := db.Model(&ToolCallRecord{}).
		Select("name, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("name").
		Order("count DESC").
		Limit(10). // Limit to top 10 tools
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = result.Name
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

// GetUniqueUsersPerDay returns the number of unique users per day
func GetUniqueUsersPerDay(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		Date  string
		Count int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, COUNT(DISTINCT user_id) as count").
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
		chartData.Labels[i] = result.Date
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

// GetTokenUsagePerUser returns the total token usage for each user
func GetTokenUsagePerUser(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		UserID uint
		Tokens int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("user_id, SUM(total_tokens) as tokens").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("user_id").
		Order("tokens DESC").
		Limit(10). // Limit to top 10 users
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = getUserName(db, result.UserID)
		chartData.Data[i] = float64(result.Tokens)
	}

	return chartData, nil
}

// GetTokenUsagePerApp returns the total token usage for each app
func GetTokenUsagePerApp(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		AppID  uint
		Tokens int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("app_id, SUM(total_tokens) as tokens").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("app_id").
		Order("tokens DESC").
		Limit(10). // Limit to top 10 apps
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		chartData.Labels[i] = getAppName(db, result.AppID)
		chartData.Data[i] = float64(result.Tokens)
	}

	return chartData, nil
}

// Helper function to get app name (you'll need to implement this based on your app model)
func getAppName(db *gorm.DB, appID uint) string {
	// Implement this function to retrieve the app's name
	// based on the appID. For example:
	// var app App
	// db.First(&app, appID)
	// return app.Name
	return "App " + strconv.Itoa(int(appID)) // Placeholder implementation
}

// GetTokenUsageForApp returns the token usage for a specific app over time
func GetTokenUsageForApp(db *gorm.DB, startDate, endDate time.Time, appID uint) (*ChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens").
		Where("time_stamp BETWEEN ? AND ? AND app_id = ?", startDate, endDate, appID).
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
		chartData.Labels[i] = result.Date
		chartData.Data[i] = float64(result.Tokens)
	}

	return chartData, nil
}

// GetChatInteractionsForChat returns the number of interactions for a specific chat over time
func GetChatInteractionsForChat(db *gorm.DB, startDate, endDate time.Time, chatID string) (*ChartData, error) {
	var results []struct {
		Date         string
		Interactions int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, COUNT(*) as interactions").
		Where("time_stamp BETWEEN ? AND ? AND chat_id = ?", startDate, endDate, chatID).
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
		chartData.Labels[i] = result.Date
		chartData.Data[i] = float64(result.Interactions)
	}

	return chartData, nil
}

// GetModelUsage returns the usage statistics for a specific model over time
func GetModelUsage(db *gorm.DB, startDate, endDate time.Time, modelName string) (*ChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Calls  int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, COUNT(*) as calls").
		Where("time_stamp BETWEEN ? AND ? AND name = ?", startDate, endDate, modelName).
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
		chartData.Labels[i] = result.Date
		// You can choose to return either tokens or calls here, or create a separate chart for each
		chartData.Data[i] = float64(result.Tokens)
	}

	return chartData, nil
}

// GetVendorUsage returns the usage statistics for a specific vendor over time
func GetVendorUsage(db *gorm.DB, startDate, endDate time.Time, vendor string) (*ChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Calls  int64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, COUNT(*) as calls").
		Where("time_stamp BETWEEN ? AND ? AND vendor = ?", startDate, endDate, vendor).
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
		chartData.Labels[i] = result.Date
		// You can choose to return either tokens or calls here, or create a separate chart for each
		chartData.Data[i] = float64(result.Tokens)
	}

	return chartData, nil
}

// GetTokenUsageAndCostForApp returns the token usage and total cost for a specific app over time
func GetTokenUsageAndCostForApp(db *gorm.DB, startDate, endDate time.Time, appID uint) (*MultiAxisChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Cost   float64
	}

	err := db.Model(&LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, SUM(cost) as cost").
		Where("time_stamp BETWEEN ? AND ? AND app_id = ?", startDate, endDate, appID).
		Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &MultiAxisChartData{
		Labels: make([]string, len(results)),
		Datasets: []Dataset{
			{
				Label: "Token Usage",
				Data:  make([]float64, len(results)),
				Yaxis: "y",
			},
			{
				Label: "Cost",
				Data:  make([]float64, len(results)),
				Yaxis: "y1",
			},
		},
	}

	for i, result := range results {
		chartData.Labels[i] = result.Date
		chartData.Datasets[0].Data[i] = float64(result.Tokens)
		chartData.Datasets[1].Data[i] = result.Cost
	}

	return chartData, nil
}

// MultiAxisChartData represents data for a chart with multiple y-axes
type MultiAxisChartData struct {
	Labels   []string  `json:"labels"`
	Datasets []Dataset `json:"datasets"`
}

// Dataset represents a single dataset in a multi-axis chart
type Dataset struct {
	Label string    `json:"label"`
	Data  []float64 `json:"data"`
	Yaxis string    `json:"yAxisID"`
}

// VendorModelCost represents the total cost for a specific vendor and model
type VendorModelCost struct {
	Vendor    string  `json:"vendor"`
	Model     string  `json:"model"`
	TotalCost float64 `json:"totalCost"`
	Currency  string  `json:"currency"`
}

// Update the GetTotalCostPerVendorAndModel function
func GetTotalCostPerVendorAndModel(db *gorm.DB, startDate, endDate time.Time) ([]VendorModelCost, error) {
	var results []VendorModelCost

	err := db.Model(&LLMChatRecord{}).
		Select("vendor, name as model, SUM(cost) as total_cost, currency").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("vendor, name, currency").
		Order("vendor, total_cost DESC").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}
