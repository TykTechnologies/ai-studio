// internal/cli/format.go
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rodaine/table"
	"gopkg.in/yaml.v3"
)

// PrintOutput formats and prints the output based on the configured format
func PrintOutput(data interface{}) error {
	switch GetFormat() {
	case "json":
		return printJSON(data)
	case "yaml":
		return printYAML(data)
	case "table":
		return printTable(data)
	default:
		return fmt.Errorf("unsupported format: %s", GetFormat())
	}
}

// printJSON prints data as formatted JSON
func printJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// printYAML prints data as YAML
func printYAML(data interface{}) error {
	encoder := yaml.NewEncoder(os.Stdout)
	defer encoder.Close()
	return encoder.Encode(data)
}

// printTable prints data as a formatted table
func printTable(data interface{}) error {
	if data == nil {
		fmt.Println("No data")
		return nil
	}

	// Handle slice of data for list operations
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice {
		return printCompactTable(data)
	}

	// Handle single item
	return printTableItem(data)
}

// printTableSlice prints a slice of items as a table
func printTableSlice(data interface{}) error {
	v := reflect.ValueOf(data)
	if v.Len() == 0 {
		fmt.Println("No items found")
		return nil
	}

	// Handle slice of maps specially
	if v.Len() > 0 {
		firstItem := v.Index(0).Interface()
		if _, ok := firstItem.(map[string]interface{}); ok {
			return printMapSliceAsTable(data)
		}
	}

	// Get the first item to determine headers
	firstItem := v.Index(0).Interface()
	headers := getTableHeaders(firstItem)

	// Create table writer
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print separator
	separators := make([]string, len(headers))
	for i := range separators {
		separators[i] = strings.Repeat("-", 10)
	}
	fmt.Fprintln(w, strings.Join(separators, "\t"))

	// Print rows
	for i := 0; i < v.Len(); i++ {
		item := v.Index(i).Interface()
		values := getTableValues(item, headers)
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}

	return w.Flush()
}

// printCompactTable prints data in a compact table format optimized for terminal viewing
func printCompactTable(data interface{}) error {
	v := reflect.ValueOf(data)
	if v.Len() == 0 {
		fmt.Println("No items found")
		return nil
	}

	// Handle slice of maps (JSON responses)
	if v.Len() > 0 {
		firstItem := v.Index(0).Interface()
		if itemMap, ok := firstItem.(map[string]interface{}); ok {
			return printCompactMapTable(data, itemMap)
		}
	}

	// Fallback to original table formatting for structs
	return printTableSlice(data)
}

// printCompactMapTable prints a slice of maps with compact, resource-specific formatting
func printCompactMapTable(data interface{}, sampleMap map[string]interface{}) error {
	v := reflect.ValueOf(data)

	// Determine resource type and show appropriate columns
	resourceType := detectResourceType(sampleMap)
	
	switch resourceType {
	case "llm":
		return printLLMTable(v)
	case "app":
		return printAppTable(v)
	case "credential":
		return printCredentialTable(v)
	case "token":
		return printTokenTable(v)
	case "budget":
		return printBudgetTable(v)
	case "analytics":
		return printAnalyticsTable(v)
	default:
		// Generic table for unknown types
		return printGenericMapTable(v)
	}
}

// detectResourceType determines what kind of resource we're displaying
func detectResourceType(sampleMap map[string]interface{}) string {
	// Check for distinctive fields to identify resource type (handle both cases)
	if _, hasVendor := sampleMap["vendor"]; hasVendor {
		if _, hasSlug := sampleMap["slug"]; hasSlug {
			return "llm"
		}
	}
	if _, hasVendor := sampleMap["Vendor"]; hasVendor {
		if _, hasSlug := sampleMap["Slug"]; hasSlug {
			return "llm"
		}
	}
	if _, hasOwnerEmail := sampleMap["owner_email"]; hasOwnerEmail {
		return "app"
	}
	if _, hasOwnerEmail := sampleMap["OwnerEmail"]; hasOwnerEmail {
		return "app"
	}
	if _, hasKeyID := sampleMap["key_id"]; hasKeyID {
		return "credential"
	}
	if _, hasKeyID := sampleMap["KeyID"]; hasKeyID {
		return "credential"
	}
	if _, hasScopes := sampleMap["scopes"]; hasScopes {
		return "token"
	}
	if _, hasScopes := sampleMap["Scopes"]; hasScopes {
		return "token"
	}
	if _, hasBudget := sampleMap["monthly_budget"]; hasBudget {
		if _, hasUsage := sampleMap["current_usage"]; hasUsage {
			return "budget"
		}
	}
	if _, hasBudget := sampleMap["MonthlyBudget"]; hasBudget {
		if _, hasUsage := sampleMap["CurrentUsage"]; hasUsage {
			return "budget"
		}
	}
	if _, hasRequestID := sampleMap["request_id"]; hasRequestID {
		return "analytics"
	}
	if _, hasRequestID := sampleMap["RequestID"]; hasRequestID {
		return "analytics"
	}
	return "generic"
}

// printLLMTable shows LLMs with essential columns: ID, Name, Vendor, Model, Active, Budget
func printLLMTable(v reflect.Value) error {
	tbl := table.New("ID", "NAME", "VENDOR", "MODEL", "ACTIVE", "BUDGET")
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			// Handle both lower case (API) and title case (Go struct) field names
			id := getMapValueCaseInsensitive(itemMap, []string{"id", "ID"}, "")
			name := getMapValueCaseInsensitive(itemMap, []string{"name", "Name"}, "")
			vendor := getMapValueCaseInsensitive(itemMap, []string{"vendor", "Vendor"}, "")
			model := getMapValueCaseInsensitive(itemMap, []string{"default_model", "DefaultModel"}, "")
			active := getMapValueCaseInsensitive(itemMap, []string{"is_active", "IsActive"}, false)
			budget := getMapValueCaseInsensitive(itemMap, []string{"monthly_budget", "MonthlyBudget"}, 0.0)
			
			activeStr := "❌"
			if active == true {
				activeStr = "✅"
			}
			
			budgetStr := fmt.Sprintf("$%.0f", budget)
			if budget == 0.0 {
				budgetStr = "unlimited"
			}
			
			tbl.AddRow(id, name, vendor, model, activeStr, budgetStr)
		}
	}
	
	tbl.Print()
	return nil
}

// printAppTable shows Apps with essential columns: ID, Name, Owner, Budget, Active
func printAppTable(v reflect.Value) error {
	tbl := table.New("ID", "NAME", "OWNER", "BUDGET", "ACTIVE")
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			id := getMapValueCaseInsensitive(itemMap, []string{"id", "ID"}, "")
			name := getMapValueCaseInsensitive(itemMap, []string{"name", "Name"}, "")
			owner := getMapValueCaseInsensitive(itemMap, []string{"owner_email", "OwnerEmail"}, "")
			budget := getMapValueCaseInsensitive(itemMap, []string{"monthly_budget", "MonthlyBudget"}, 0.0)
			active := getMapValueCaseInsensitive(itemMap, []string{"is_active", "IsActive"}, false)
			
			activeStr := "❌"
			if active == true {
				activeStr = "✅"
			}
			
			budgetStr := fmt.Sprintf("$%.0f", budget)
			if budget == 0.0 {
				budgetStr = "unlimited"
			}
			
			tbl.AddRow(id, name, owner, budgetStr, activeStr)
		}
	}
	
	tbl.Print()
	return nil
}

// printCredentialTable shows credentials with: ID, Name, Key ID, Active, Expires
func printCredentialTable(v reflect.Value) error {
	tbl := table.New("ID", "NAME", "KEY_ID", "ACTIVE", "EXPIRES")
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			id := getMapValue(itemMap, "id", "")
			name := getMapValue(itemMap, "name", "")
			keyID := getMapValue(itemMap, "key_id", "")
			active := getMapValue(itemMap, "is_active", false)
			expiresAt := getMapValue(itemMap, "expires_at", "")
			
			activeStr := "❌"
			if active == true {
				activeStr = "✅"
			}
			
			expiresStr := "never"
			if expiresAt != "" && expiresAt != nil {
				if expTime, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", expiresAt)); err == nil {
					expiresStr = expTime.Format("2006-01-02")
				}
			}
			
			tbl.AddRow(id, name, keyID, activeStr, expiresStr)
		}
	}
	
	tbl.Print()
	return nil
}

// printTokenTable shows tokens with: ID, Name, App ID, Scopes, Expires
func printTokenTable(v reflect.Value) error {
	tbl := table.New("ID", "NAME", "APP_ID", "SCOPES", "EXPIRES")
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			id := getMapValue(itemMap, "id", "")
			name := getMapValue(itemMap, "name", "")
			appID := getMapValue(itemMap, "app_id", "")
			scopes := getMapValue(itemMap, "scopes", []interface{}{})
			expiresAt := getMapValue(itemMap, "expires_at", "")
			
			scopesStr := "none"
			if scopeSlice, ok := scopes.([]interface{}); ok && len(scopeSlice) > 0 {
				scopeStrs := make([]string, len(scopeSlice))
				for j, scope := range scopeSlice {
					scopeStrs[j] = fmt.Sprintf("%v", scope)
				}
				scopesStr = strings.Join(scopeStrs, ",")
			}
			
			expiresStr := "never"
			if expiresAt != "" && expiresAt != nil {
				if expTime, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", expiresAt)); err == nil {
					expiresStr = expTime.Format("2006-01-02")
				}
			}
			
			tbl.AddRow(id, name, appID, scopesStr, expiresStr)
		}
	}
	
	tbl.Print()
	return nil
}

// printBudgetTable shows budget info with: App ID, Current Usage, Budget, Remaining, % Used
func printBudgetTable(v reflect.Value) error {
	tbl := table.New("APP_ID", "USAGE", "BUDGET", "REMAINING", "% USED")
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			appID := getMapValue(itemMap, "app_id", "")
			usage := getMapValue(itemMap, "current_usage", 0.0)
			budget := getMapValue(itemMap, "monthly_budget", 0.0)
			remaining := getMapValue(itemMap, "remaining_budget", 0.0)
			percent := getMapValue(itemMap, "percentage_used", 0.0)
			
			usageStr := fmt.Sprintf("$%.2f", usage)
			budgetStr := fmt.Sprintf("$%.2f", budget)
			remainingStr := fmt.Sprintf("$%.2f", remaining)
			percentStr := fmt.Sprintf("%.1f%%", percent)
			
			tbl.AddRow(appID, usageStr, budgetStr, remainingStr, percentStr)
		}
	}
	
	tbl.Print()
	return nil
}

// printAnalyticsTable shows analytics with: ID, Endpoint, Method, Status, Tokens, Cost, Latency
func printAnalyticsTable(v reflect.Value) error {
	tbl := table.New("ID", "ENDPOINT", "METHOD", "STATUS", "TOKENS", "COST", "LATENCY")
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			id := getMapValue(itemMap, "id", "")
			endpoint := getMapValue(itemMap, "endpoint", "")
			method := getMapValue(itemMap, "method", "")
			status := getMapValue(itemMap, "status_code", 0)
			tokens := getMapValue(itemMap, "total_tokens", 0)
			cost := getMapValue(itemMap, "cost", 0.0)
			latency := getMapValue(itemMap, "latency_ms", 0)
			
			// Truncate long endpoints
			endpointStr := fmt.Sprintf("%v", endpoint)
			if len(endpointStr) > 30 {
				endpointStr = endpointStr[:27] + "..."
			}
			endpoint = endpointStr
			
			costStr := fmt.Sprintf("$%.3f", cost)
			latencyStr := fmt.Sprintf("%dms", latency)
			
			tbl.AddRow(id, endpoint, method, status, tokens, costStr, latencyStr)
		}
	}
	
	tbl.Print()
	return nil
}

// printGenericMapTable prints a generic table for unknown resource types
func printGenericMapTable(v reflect.Value) error {
	if v.Len() == 0 {
		fmt.Println("No items found")
		return nil
	}

	// Get common keys from first item
	firstItem := v.Index(0).Interface().(map[string]interface{})
	
	// Show essential fields that most resources have
	essentialFields := []string{"id", "name", "is_active", "created_at"}
	headers := []string{}
	
	for _, field := range essentialFields {
		if _, exists := firstItem[field]; exists {
			headers = append(headers, strings.ToUpper(field))
		}
	}
	
	if len(headers) == 0 {
		// Fallback to all fields if no essential fields found
		for key := range firstItem {
			headers = append(headers, strings.ToUpper(key))
		}
	}

	// Convert headers to interface slice for table.New
	headerInterfaces := make([]interface{}, len(headers))
	for i, h := range headers {
		headerInterfaces[i] = h
	}
	
	tbl := table.New(headerInterfaces...)
	
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			values := make([]interface{}, len(headers))
			for j, header := range headers {
				key := strings.ToLower(header)
				if value, exists := itemMap[key]; exists {
					values[j] = formatInterfaceValue(value)
				} else {
					values[j] = ""
				}
			}
			tbl.AddRow(values...)
		}
	}
	
	tbl.Print()
	return nil
}

// getMapValue safely extracts a value from a map with type conversion
func getMapValue(m map[string]interface{}, key string, defaultValue interface{}) interface{} {
	if value, exists := m[key]; exists && value != nil {
		return value
	}
	return defaultValue
}

// getMapValueCaseInsensitive tries multiple key variations and returns the first match
func getMapValueCaseInsensitive(m map[string]interface{}, keys []string, defaultValue interface{}) interface{} {
	for _, key := range keys {
		if value, exists := m[key]; exists && value != nil {
			return value
		}
	}
	return defaultValue
}

// printMapSliceAsTable prints a slice of maps as a table
func printMapSliceAsTable(data interface{}) error {
	v := reflect.ValueOf(data)
	if v.Len() == 0 {
		fmt.Println("No items found")
		return nil
	}

	// Collect all unique keys from all maps
	allKeys := make(map[string]bool)
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			for key := range itemMap {
				allKeys[key] = true
			}
		}
	}

	// Convert to sorted slice
	headers := make([]string, 0, len(allKeys))
	for key := range allKeys {
		headers = append(headers, key)
	}

	// Create table writer
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print separator
	separators := make([]string, len(headers))
	for i := range separators {
		separators[i] = strings.Repeat("-", 10)
	}
	fmt.Fprintln(w, strings.Join(separators, "\t"))

	// Print rows
	for i := 0; i < v.Len(); i++ {
		if itemMap, ok := v.Index(i).Interface().(map[string]interface{}); ok {
			values := make([]string, len(headers))
			for j, header := range headers {
				if value, exists := itemMap[header]; exists {
					values[j] = formatInterfaceValue(value)
				} else {
					values[j] = ""
				}
			}
			fmt.Fprintln(w, strings.Join(values, "\t"))
		}
	}

	return w.Flush()
}

// printTableItem prints a single item as a table
func printTableItem(data interface{}) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Handle map[string]interface{} specially
	if dataMap, ok := data.(map[string]interface{}); ok {
		for key, value := range dataMap {
			fmt.Fprintf(w, "%s:\t%v\n", key, formatInterfaceValue(value))
		}
		return w.Flush()
	}

	// Handle structs with reflection
	headers := getTableHeaders(data)
	values := getTableValues(data, headers)

	// Print as key-value pairs
	for i, header := range headers {
		if i < len(values) {
			fmt.Fprintf(w, "%s:\t%s\n", header, values[i])
		}
	}

	return w.Flush()
}

// getTableHeaders extracts field names for table headers
func getTableHeaders(data interface{}) []string {
	v := reflect.ValueOf(data)
	t := reflect.TypeOf(data)

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}

	var headers []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		
		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Use json tag name if available, otherwise field name
		headerName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" && parts[0] != "-" {
				headerName = strings.ToUpper(parts[0])
			}
		}

		headers = append(headers, headerName)
	}

	return headers
}

// getTableValues extracts field values for table rows
func getTableValues(data interface{}, headers []string) []string {
	v := reflect.ValueOf(data)

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var values []string
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		
		// Skip unexported fields
		fieldType := v.Type().Field(i)
		if !fieldType.IsExported() {
			continue
		}

		// Format the value
		value := formatFieldValue(field)
		values = append(values, value)
	}

	return values
}

// formatFieldValue formats a field value for display
func formatFieldValue(v reflect.Value) string {
	if !v.IsValid() {
		return "<nil>"
	}

	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return "<nil>"
		}
		return formatFieldValue(v.Elem())
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%.2f", v.Float())
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return "[]"
		}
		// For arrays/slices, show count
		return fmt.Sprintf("[%d items]", v.Len())
	case reflect.Map:
		if v.Len() == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d keys}", v.Len())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	fmt.Printf("✅ %s\n", message)
}

// formatInterfaceValue formats an interface{} value for display
func formatInterfaceValue(value interface{}) string {
	if value == nil {
		return "<nil>"
	}

	switch v := value.(type) {
	case string:
		return v
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%v", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", v)
	case float32, float64:
		return fmt.Sprintf("%.2f", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case []interface{}:
		if len(v) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", len(v))
	case map[string]interface{}:
		if len(v) == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d keys}", len(v))
	default:
		return fmt.Sprintf("%v", value)
	}
}

// PrintError prints an error message
func PrintError(err error) {
	fmt.Printf("❌ Error: %v\n", err)
}

// PrintWarning prints a warning message
func PrintWarning(message string) {
	fmt.Printf("⚠️  %s\n", message)
}