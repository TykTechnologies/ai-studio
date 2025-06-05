package analytics

import (
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

type ChartData struct {
	Labels []string  `json:"labels"`
	Data   []float64 `json:"data"`
	Cost   []float64 `json:"cost,omitempty"`
}

// Custom struct for scanning budget stats
type budgetStats struct {
	LLMID           uint
	Name            string
	MonthlyUsage    float64
	TotalCost       float64
	TotalTokens     int64
	MonthlyBudget   *float64
	BudgetStartDate string // Store as string and convert in toBudgetUsage
	UserID          uint
	UserEmail       string
}

// Convert string to *time.Time, handling empty strings and invalid formats
func parseDateTime(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02 15:04:05", dateStr)
	if err != nil {
		return nil
	}
	return &t
}

// Convert budgetStats to models.BudgetUsage
func (bs budgetStats) toBudgetUsage(entityType string) models.BudgetUsage {
	usage := float64(0)
	if bs.MonthlyBudget != nil && *bs.MonthlyBudget > 0 {
		usage = (bs.MonthlyUsage / *bs.MonthlyBudget) * 100
	}

	budgetStartDate := parseDateTime(bs.BudgetStartDate)

	var userEmail *string
	if bs.UserEmail != "" {
		userEmail = &bs.UserEmail
	}

	return models.BudgetUsage{
		EntityID:        bs.LLMID,
		Name:            bs.Name,
		EntityType:      entityType,
		Budget:          bs.MonthlyBudget,
		Spent:           bs.MonthlyUsage,
		Usage:           usage,
		TotalCost:       bs.TotalCost,
		TotalTokens:     bs.TotalTokens,
		BudgetStartDate: budgetStartDate,
		UserID:          bs.UserID,
		UserEmail:       userEmail,
	}
}

// GetChatRecordsPerDay returns the total number of chat records per day for a given time period
func GetChatRecordsPerDay(db *gorm.DB, startDate, endDate *time.Time) (*ChartData, error) {
	var results []struct {
		Date  string
		Count int64
	}

	err := db.Model(&models.LLMChatRecord{}).
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
		chartData.Labels[i] = result.Date
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

func GetToolCallsPerDay(db *gorm.DB, startDate, endDate time.Time) (*ChartData, error) {
	var results []struct {
		Date  string
		Count int64
	}

	err := db.Model(&models.ToolCallRecord{}).
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
		chartData.Labels[i] = result.Date
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

	err := db.Model(&models.LLMChatRecord{}).
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
		chartData.Labels[i] = getUserName(db, result.UserID)
		chartData.Data[i] = float64(result.Count)
	}

	return chartData, nil
}

// Helper function to get user name
func getUserName(db *gorm.DB, userID uint) string {
	strUserID := strconv.Itoa(int(userID))
	return "User " + strUserID
}

// GetCostAnalysis returns the total cost per day for each currency and interaction type
func GetCostAnalysis(db *gorm.DB, startDate, endDate time.Time, interactionType *models.InteractionType) (map[string]*ChartData, error) {
	var results []struct {
		Date     string
		Currency string
		Cost     float64
	}

	query := db.Model(&models.LLMChatRecord{}).
		Select("DATE(time_stamp) as date, currency, SUM(cost) as cost").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate)

	if interactionType != nil {
		query = query.Where("interaction_type = ?", *interactionType)
	}

	err := query.Group("DATE(time_stamp), currency").
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
		chartDataMap[result.Currency].Data = append(chartDataMap[result.Currency].Data, result.Cost/10000)
	}

	return chartDataMap, nil
}

// GetMostUsedLLMModels returns the usage count for each LLM model
func GetMostUsedLLMModels(db *gorm.DB, startDate, endDate time.Time, interactionType *models.InteractionType) (*ChartData, error) {
	var results []struct {
		Name  string
		Count int64
	}

	query := db.Model(&models.LLMChatRecord{}).
		Select("COALESCE(NULLIF(name, ''), 'Unknown') as name, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate)

	if interactionType != nil {
		query = query.Where("interaction_type = ?", *interactionType)
	}

	err := query.Group("COALESCE(NULLIF(name, ''), 'Unknown')").
		Order("count DESC").
		Limit(10).
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

	err := db.Model(&models.ToolCallRecord{}).
		Select("name, COUNT(*) as count").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate).
		Group("name").
		Order("count DESC").
		Limit(10).
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

	err := db.Model(&models.LLMChatRecord{}).
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
func GetTokenUsagePerUser(db *gorm.DB, startDate, endDate time.Time, interactionType *models.InteractionType) (*ChartData, error) {
	var results []struct {
		UserID uint
		Tokens int64
	}

	query := db.Model(&models.LLMChatRecord{}).
		Select("user_id, SUM(total_tokens) as tokens").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate)

	if interactionType != nil {
		query = query.Where("interaction_type = ?", *interactionType)
	}

	err := query.Group("user_id").
		Order("tokens DESC").
		Limit(10).
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
func GetTokenUsagePerApp(db *gorm.DB, startDate, endDate time.Time, interactionType *models.InteractionType) (*ChartData, error) {
	var results []struct {
		AppID  uint
		Tokens int64
	}

	query := db.Model(&models.LLMChatRecord{}).
		Select("app_id, SUM(total_tokens) as tokens").
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate)

	if interactionType != nil {
		query = query.Where("interaction_type = ?", *interactionType)
	}

	err := query.Group("app_id").
		Order("tokens DESC").
		Limit(10).
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

// Helper function to get app name
func getAppName(db *gorm.DB, appID uint) string {
	return "App " + strconv.Itoa(int(appID))
}

// GetTokenUsageForApp returns the token usage for a specific app over time
func GetTokenUsageForApp(db *gorm.DB, startDate, endDate time.Time, appID uint) (*ChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Cost   float64
	}

	err := db.Model(&models.LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, SUM(cost) as cost").
		Where("time_stamp BETWEEN ? AND ? AND app_id = ?", startDate, endDate, appID).
		Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	tokenData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	costData := &ChartData{
		Labels: make([]string, len(results)),
		Data:   make([]float64, len(results)),
	}

	for i, result := range results {
		tokenData.Labels[i] = result.Date
		tokenData.Data[i] = float64(result.Tokens)
		costData.Labels[i] = result.Date
		costData.Data[i] = result.Cost/10000
	}

	return &ChartData{
		Labels: tokenData.Labels,
		Data:   tokenData.Data,
		Cost:   costData.Data,
	}, nil
}

// GetChatInteractionsForChat returns the number of interactions for a specific chat over time
func GetChatInteractionsForChat(db *gorm.DB, startDate, endDate time.Time, chatID string) (*ChartData, error) {
	var results []struct {
		Date         string
		Interactions int64
	}

	err := db.Model(&models.LLMChatRecord{}).
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

// GetAppInteractionsOverTime returns the number of LLM interactions for a specific app over time
func GetAppInteractionsOverTime(db *gorm.DB, startDate, endDate time.Time, appID uint) (*ChartData, error) {
	var results []struct {
		Date         string
		Interactions int64
	}

	err := db.Model(&models.LLMChatRecord{}).
		Select("DATE(time_stamp) as date, COUNT(*) as interactions").
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
		chartData.Data[i] = float64(result.Interactions)
	}

	return chartData, nil
}

// GetModelUsage returns the usage statistics for a specific model over time
func GetModelUsage(db *gorm.DB, startDate, endDate time.Time, modelName string) (*ChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Cost   float64
		Calls  int64
	}

	err := db.Model(&models.LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, SUM(cost) as cost, COUNT(*) as calls").
		Where("time_stamp BETWEEN ? AND ? AND name = ?", startDate, endDate, modelName).
		Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	response := struct {
		Labels     []string  `json:"labels"`
		TokenUsage []float64 `json:"token_usage"`
		Cost       []float64 `json:"cost"`
	}{
		Labels:     make([]string, len(results)),
		TokenUsage: make([]float64, len(results)),
		Cost:       make([]float64, len(results)),
	}

	for i, result := range results {
		response.Labels[i] = result.Date
		response.TokenUsage[i] = float64(result.Tokens)
		response.Cost[i] = result.Cost/10000
	}

	return &ChartData{
		Labels: response.Labels,
		Data:   response.TokenUsage,
		Cost:   response.Cost,
	}, nil
}

// GetVendorUsage returns the usage statistics for a specific vendor over time
func GetVendorUsage(db *gorm.DB, startDate, endDate time.Time, vendor string, llmID *uint) (*ChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Cost   float64
		Calls  int64
	}

	query := db.Model(&models.LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, SUM(cost) as cost, COUNT(*) as calls").
		Where("time_stamp BETWEEN ? AND ? AND vendor = ?", startDate, endDate, vendor)

	if llmID != nil {
		query = query.Where("llm_id = ?", *llmID)
	}

	err := query.Debug().Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	response := struct {
		Labels     []string  `json:"labels"`
		TokenUsage []float64 `json:"token_usage"`
		Cost       []float64 `json:"cost"`
	}{
		Labels:     make([]string, len(results)),
		TokenUsage: make([]float64, len(results)),
		Cost:       make([]float64, len(results)),
	}

	for i, result := range results {
		response.Labels[i] = result.Date
		response.TokenUsage[i] = float64(result.Tokens)
		response.Cost[i] = result.Cost/10000
	}

	return &ChartData{
		Labels: response.Labels,
		Data:   response.TokenUsage,
		Cost:   response.Cost,
	}, nil
}

// GetUsage returns token usage and cost data based on provided filters
func GetUsage(db *gorm.DB, startDate, endDate time.Time, vendor string, llmID, appID *uint, interactionType *models.InteractionType) (*models.MultiAxisChartData, error) {
	var results []struct {
		Date             string
		Tokens           int64
		Cost             float64
		PromptTokens     int64
		ResponseTokens   int64
		CacheWriteTokens int64
		CacheReadTokens  int64
	}

	query := db.Model(&models.LLMChatRecord{}).
		Select(`
			DATE(time_stamp) as date, 
			SUM(total_tokens) as tokens, 
			SUM(cost) as cost,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(response_tokens), 0) as response_tokens,
			COALESCE(SUM(COALESCE(cache_write_prompt_tokens, 0)), 0) as cache_write_tokens,
			COALESCE(SUM(COALESCE(cache_read_prompt_tokens, 0)), 0) as cache_read_tokens
		`).
		Where("time_stamp BETWEEN ? AND ?", startDate, endDate)

	if vendor != "" {
		query = query.Where("vendor = ?", vendor)
	}
	if llmID != nil {
		query = query.Where("llm_id = ?", *llmID)
	}
	if appID != nil {
		query = query.Where("app_id = ?", *appID)
	}
	if interactionType != nil {
		query = query.Where("interaction_type = ?", *interactionType)
	}

	err := query.Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &models.MultiAxisChartData{
		Labels: make([]string, len(results)),
		Datasets: []models.Dataset{
			{
				Label: "Total Tokens",
				Data:  make([]float64, len(results)),
				Yaxis: "y",
			},
			{
				Label: "Cost",
				Data:  make([]float64, len(results)),
				Yaxis: "y1",
			},
			{
				Label: "Prompt Tokens",
				Data:  make([]float64, len(results)),
				Yaxis: "y",
			},
			{
				Label: "Response Tokens",
				Data:  make([]float64, len(results)),
				Yaxis: "y",
			},
			{
				Label: "Cache Write Tokens",
				Data:  make([]float64, len(results)),
				Yaxis: "y",
			},
			{
				Label: "Cache Read Tokens",
				Data:  make([]float64, len(results)),
				Yaxis: "y",
			},
		},
	}

	for i, result := range results {
		chartData.Labels[i] = result.Date
		chartData.Datasets[0].Data[i] = float64(result.Tokens)
		chartData.Datasets[1].Data[i] = result.Cost/10000
		chartData.Datasets[2].Data[i] = float64(result.PromptTokens)
		chartData.Datasets[3].Data[i] = float64(result.ResponseTokens)
		chartData.Datasets[4].Data[i] = float64(result.CacheWriteTokens)
		chartData.Datasets[5].Data[i] = float64(result.CacheReadTokens)
	}

	return chartData, nil
}

// GetTokenUsageAndCostForApp returns the token usage and total cost for a specific app over time
func GetTokenUsageAndCostForApp(db *gorm.DB, startDate, endDate time.Time, appID uint) (*models.MultiAxisChartData, error) {
	var results []struct {
		Date   string
		Tokens int64
		Cost   float64
	}

	err := db.Model(&models.LLMChatRecord{}).
		Select("DATE(time_stamp) as date, SUM(total_tokens) as tokens, SUM(cost) as cost").
		Where("time_stamp BETWEEN ? AND ? AND app_id = ?", startDate, endDate, appID).
		Group("DATE(time_stamp)").
		Order("date").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	chartData := &models.MultiAxisChartData{
		Labels: make([]string, len(results)),
		Datasets: []models.Dataset{
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
		chartData.Datasets[1].Data[i] = result.Cost/10000
	}

	return chartData, nil
}

// VendorModelCost represents the total cost for a specific vendor and model
type VendorModelCost struct {
	Model            string  `json:"model"`
	ModelPriceID     *uint   `json:"modelPriceId"`
	TotalCost        float64 `json:"totalCost"`
	PromptCost       float64 `json:"promptCost"`
	ResponseCost     float64 `json:"responseCost"`
	CacheWriteCost   float64 `json:"cacheWriteCost"`
	CacheReadCost    float64 `json:"cacheReadCost"`
	TotalTokens      int64   `json:"totalTokens"` // Total of all token types
	PromptTokens     int64   `json:"promptTokens"`
	ResponseTokens   int64   `json:"responseTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
}

// GetTotalCostPerVendorAndModel returns the total cost per vendor and model with detailed breakdowns
func GetTotalCostPerVendorAndModel(db *gorm.DB, startDate, endDate time.Time, interactionType *models.InteractionType, llmID *uint) ([]VendorModelCost, error) {
	var results []VendorModelCost

	query := db.Table("llm_chat_records").
		Joins("LEFT JOIN model_prices ON model_prices.model_name = llm_chat_records.name AND model_prices.vendor = llm_chat_records.vendor").
		Select(`
			COALESCE(NULLIF(llm_chat_records.name, ''), 'Unknown') as model,
			model_prices.id as model_price_id,
			llm_chat_records.vendor,
			SUM(llm_chat_records.cost) as total_cost,
			SUM(COALESCE(llm_chat_records.prompt_tokens * COALESCE(model_prices.cpit, 0) * 10000, 0)) as prompt_cost,
			SUM(COALESCE(llm_chat_records.response_tokens * COALESCE(model_prices.cpt, 0) * 10000, 0)) as response_cost,
			SUM(COALESCE(llm_chat_records.cache_write_prompt_tokens * COALESCE(model_prices.cache_write_pt, 0) * 10000, 0)) as cache_write_cost,
			SUM(COALESCE(llm_chat_records.cache_read_prompt_tokens * COALESCE(model_prices.cache_read_pt, 0) * 10000, 0)) as cache_read_cost,
			SUM(COALESCE(llm_chat_records.total_tokens, 0)) as total_tokens,
			SUM(COALESCE(llm_chat_records.prompt_tokens, 0)) as prompt_tokens,
			SUM(COALESCE(llm_chat_records.response_tokens, 0)) as response_tokens,
			SUM(COALESCE(llm_chat_records.cache_write_prompt_tokens, 0)) as cache_write_tokens,
			SUM(COALESCE(llm_chat_records.cache_read_prompt_tokens, 0)) as cache_read_tokens
		`).
		Where("llm_chat_records.time_stamp BETWEEN ? AND ? AND llm_chat_records.name IS NOT NULL", startDate, endDate)

	if interactionType != nil {
		query = query.Where("interaction_type = ?", *interactionType)
	}

	if llmID != nil {
		query = query.Where("llm_id = ?", *llmID)
	}

	err := query.Group("COALESCE(NULLIF(llm_chat_records.name, ''), 'Unknown'), model_prices.id, llm_chat_records.vendor").
		Order("total_cost DESC").
		Find(&results).Error

	// Apply division by 10000 after fetching results
	for i := range results {
		results[i].TotalCost = results[i].TotalCost / 10000
		results[i].PromptCost = results[i].PromptCost / 10000
		results[i].ResponseCost = results[i].ResponseCost / 10000
		results[i].CacheWriteCost = results[i].CacheWriteCost / 10000
		results[i].CacheReadCost = results[i].CacheReadCost / 10000
	}

	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetChatLogsForChatID retrieves all chat log entries for a specific chat ID
func GetChatLogsForChatID(db *gorm.DB, chatID uint) ([]models.LLMChatLogEntry, error) {
	var chatLogs []models.LLMChatLogEntry

	err := db.Where("chat_id = ?", chatID).
		Order("time_stamp ASC").
		Find(&chatLogs).Error

	if err != nil {
		return nil, err
	}

	return chatLogs, nil
}

func minTime(times ...time.Time) time.Time {
	if len(times) == 0 {
		panic("no times provided")
	}
	m := times[0]
	for _, t := range times[1:] {
		if t.Before(m) {
			m = t
		}
	}
	return m
}

func maxTime(times ...time.Time) time.Time {
	if len(times) == 0 {
		panic("no times provided")
	}
	m := times[0]
	for _, t := range times[1:] {
		if t.After(m) {
			m = t
		}
	}
	return m
}

// GetBudgetUsage returns usage statistics for all LLMs and Apps that have costs, with optional date range
func GetBudgetUsage(db *gorm.DB, startDate, endDate *time.Time, llmID *uint) ([]models.BudgetUsage, error) {
	var result []models.BudgetUsage

	// Get current month's start and end dates for budget calculation
	now := time.Now().UTC()

	// Set start of month to beginning of day (00:00:00) UTC
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Set end of month to end of the last day (23:59:59) UTC
	lastDay := startOfMonth.AddDate(0, 1, -1)
	endOfMonth := time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 23, 59, 59, 0, time.UTC)

	// Use provided date range for total cost calculation, or default to current month
	costStartDate := startOfMonth
	costEndDate := endOfMonth
	if startDate != nil {
		// Keep start date at beginning of day UTC
		costStartDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	}
	if endDate != nil {
		// Set end date to end of day (23:59:59) UTC
		costEndDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, time.UTC)
	}

	// Get LLM usage statistics
	var llmStats []budgetStats

	minDate := minTime(startOfMonth, endOfMonth, costStartDate, costEndDate)
	maxDate := maxTime(startOfMonth, endOfMonth, costStartDate, costEndDate)

	// Get LLM usage with proper handling of NULL and 0 values
	llmQuery := db.Table("llm_chat_records").
		Select(`
			llm_chat_records.llm_id,
			COALESCE(llms.name, NULLIF(llm_chat_records.vendor, ''), 'Unknown') AS name,
			SUM(CASE WHEN time_stamp BETWEEN ? AND ? THEN COALESCE(cost, 0) ELSE 0 END) AS monthly_usage,
			SUM(CASE WHEN time_stamp BETWEEN ? AND ? THEN COALESCE(cost, 0) ELSE 0 END) AS total_cost,
			SUM(CASE WHEN time_stamp BETWEEN ? AND ? THEN COALESCE(total_tokens, 0) ELSE 0 END) AS total_tokens,
			llms.monthly_budget,
			llms.budget_start_date
	`, startOfMonth, endOfMonth, costStartDate, costEndDate, costStartDate, costEndDate).
		Joins("LEFT JOIN llms ON llm_chat_records.llm_id = llms.id AND llms.deleted_at IS NULL").
		Where("time_stamp BETWEEN ? AND ?", minDate, maxDate).
		Group("llm_chat_records.llm_id, COALESCE(llms.name, NULLIF(llm_chat_records.vendor, ''), 'Unknown'), llms.monthly_budget, llms.budget_start_date")

	if llmID != nil {
		llmQuery = llmQuery.Where("llms.id = ?", *llmID)
	}

	if err := llmQuery.Debug().Find(&llmStats).Error; err != nil {
		return nil, err
	}
	println("LEN:", len(llmStats))

	for _, stat := range llmStats {
		stat.MonthlyUsage = stat.MonthlyUsage / 10000
		stat.TotalCost = stat.TotalCost / 10000
		result = append(result, stat.toBudgetUsage("LLM"))
	}

	// Get App usage statistics
	var appStats []budgetStats

	// Get App usage with proper handling of NULL and 0 values
	if err := db.Table("llm_chat_records").
		Select(`
        COALESCE(llm_chat_records.app_id, 0) AS llm_id,
        COALESCE(apps.name, 'Tyk AI Studio Chat') AS name,
        SUM(CASE WHEN time_stamp BETWEEN ? AND ? THEN COALESCE(cost, 0) ELSE 0 END) AS monthly_usage,
        SUM(CASE WHEN time_stamp BETWEEN ? AND ? THEN COALESCE(cost, 0) ELSE 0 END) AS total_cost,
        SUM(CASE WHEN time_stamp BETWEEN ? AND ? THEN COALESCE(total_tokens, 0) ELSE 0 END) AS total_tokens,
        apps.monthly_budget,
        apps.budget_start_date,
        COALESCE(apps.user_id, 0) AS user_id,
        COALESCE(users.email, CASE WHEN apps.id IS NOT NULL THEN 'Unknown User' ELSE NULL END) AS user_email
    `, startOfMonth, endOfMonth, costStartDate, costEndDate, costStartDate, costEndDate).
		Joins("LEFT JOIN apps ON llm_chat_records.app_id = apps.id AND apps.deleted_at IS NULL").
		Where("time_stamp BETWEEN ? AND ?", minDate, maxDate).
		Joins("LEFT JOIN users ON apps.user_id = users.id AND users.deleted_at IS NULL").
		Group("COALESCE(llm_chat_records.app_id, 0), COALESCE(apps.name, 'Tyk AI Studio Chat'), apps.monthly_budget, apps.budget_start_date, COALESCE(apps.user_id, 0), user_email").
		Find(&appStats).Error; err != nil {
		return nil, err
	}

	for _, stat := range appStats {
		stat.MonthlyUsage = stat.MonthlyUsage / 10000
		stat.TotalCost = stat.TotalCost / 10000
		result = append(result, stat.toBudgetUsage("App"))
	}

	return result, nil
}

// GetProxyLogsForAppID returns paginated proxy logs for a specific app
func GetProxyLogsForAppID(db *gorm.DB, startDate, endDate time.Time, appID uint, page, pageSize int) ([]models.ProxyLog, int64, error) {
	var proxyLogs []models.ProxyLog
	var totalCount int64

	// Count total records
	err := db.Model(&models.ProxyLog{}).
		Where("app_id = ? AND time_stamp BETWEEN ? AND ?", appID, startDate, endDate).
		Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	// Retrieve paginated records
	offset := (page - 1) * pageSize
	err = db.Where("app_id = ? AND time_stamp BETWEEN ? AND ?", appID, startDate, endDate).
		Order("time_stamp DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&proxyLogs).Error

	if err != nil {
		return nil, 0, err
	}

	return proxyLogs, totalCount, nil
}

// GetProxyLogsForLLM returns paginated proxy logs for a specific LLM by filtering on vendor
func GetProxyLogsForLLM(db *gorm.DB, startDate, endDate time.Time, llmID uint, page, pageSize int) ([]models.ProxyLog, int64, error) {
	var proxyLogs []models.ProxyLog
	var totalCount int64

	// Get the LLM's vendor
	var llm struct {
		Vendor string
	}
	if err := db.Table("llms").Select("vendor").Where("id = ?", llmID).Scan(&llm).Error; err != nil {
		return nil, 0, err
	}

	// Filter proxy_logs by vendor and date range
	query := db.Model(&models.ProxyLog{}).
		Where("vendor = ? AND time_stamp BETWEEN ? AND ?", llm.Vendor, startDate, endDate)

	// Count total records
	err := query.Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	// Retrieve paginated records
	offset := (page - 1) * pageSize
	err = query.Order("time_stamp DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&proxyLogs).Error

	if err != nil {
		return nil, 0, err
	}

	return proxyLogs, totalCount, nil
}
