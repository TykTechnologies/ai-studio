// internal/cli/format.go
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

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
		return printTableSlice(data)
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