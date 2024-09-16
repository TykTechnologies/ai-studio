package scripting

import (
	"testing"
)

func TestRunFilter(t *testing.T) {
	tests := []struct {
		name       string
		sourceCode string
		payload    string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "successful filter - return true",
			sourceCode: `
				filter := func(p) {
					return true
				}
				result := filter(payload)
			`,
			payload: "hello",
			wantErr: false,
		},
		{
			name: "successful filter - return false",
			sourceCode: `
				filter := func(p) {
					return false
				}
				result := filter(payload)
			`,
			payload: "world",
			wantErr: true,
			errMsg:  "filter returned false",
		},
		{
			name: "syntax error",
			sourceCode: `
				filter := func(p) {
					return p == "hello"
				}
				result := filter(payload
			`,
			payload: "hello",
			wantErr: true,
			errMsg:  "compilation error",
		},
		// {
		// 	name: "runtime error",
		// 	sourceCode: `
		// 		filter := func(p) {
		// 			return 1 / 0 > 0
		// 		}
		// 		result := filter(payload)
		// 	`,
		// 	payload: "hello",
		// 	wantErr: true,
		// 	errMsg:  "runtime error",
		// },
		{
			name: "missing result",
			sourceCode: `
				filter := func(p) {
					return p == "hello"
				}
				// result is not set
			`,
			payload: "hello",
			wantErr: true,
		},
		{
			name: "non-boolean result",
			sourceCode: `
				filter := func(p) {
					return 42
				}
				result := filter(payload)
			`,
			payload: "hello",
			wantErr: false, // The implementation converts non-boolean to boolean
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunFilter(tt.sourceCode, tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if got := err.Error(); got[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("RunFilter() error message = %v, want %v", got, tt.errMsg)
				}
			}
		})
	}
}

func TestScriptRunnerConcurrency(t *testing.T) {
	sourceCode := `
		filter := func(p) {
			return true
		}
		result := filter(payload)
	`

	runner := NewScriptRunner([]byte(sourceCode))
	concurrentRuns := 100

	done := make(chan bool)
	for i := 0; i < concurrentRuns; i++ {
		go func() {
			err := runner.RunFilter("hello")
			if err != nil {
				t.Errorf("Concurrent runFilter() error = %v", err)
			}
			done <- true
		}()
	}

	for i := 0; i < concurrentRuns; i++ {
		<-done
	}
}
