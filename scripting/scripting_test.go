package scripting

import (
	"fmt"
	"os"
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
			err := RunFilter(tt.sourceCode, tt.payload, nil)
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
			err := runner.RunFilter("hello", nil)
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

func TestRunScriptFile(t *testing.T) {
	sourceCode, _ := os.ReadFile("examples/ner_blob_service.tengo")
	runner := NewScriptRunner(sourceCode)
	out, err := runner.RunMiddleware(test_data, nil)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FINAL OUTPUT")
	fmt.Println("===================================================================")
	fmt.Println(out)
	fmt.Println("===================================================================")

}

var test_data = `
{"expand":"schema,names","startAt":0,"maxResults":50,"total":7,"issues":[{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45156","self":"https://tyktech.atlassian.net/rest/api/3/issue/45156","key":"TD-3482","fields":{"summary":"Allow 'Artnight' to rollback to v5.5.0","created":"2024-11-13T20:27:56.526+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/10005","description":"","iconUrl":"https://tyktech.atlassian.net/","name":"Done","id":"10005","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/3","id":3,"key":"done","colorName":"green","name":"Done"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45155","self":"https://tyktech.atlassian.net/rest/api/3/issue/45155","key":"TD-3481","fields":{"summary":"Create runbook for bulk custom domain add","created":"2024-11-13T19:31:41.972+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/10003","description":"","iconUrl":"https://tyktech.atlassian.net/","name":"New","id":"10003","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/2","id":2,"key":"new","colorName":"blue-gray","name":"To Do"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45154","self":"https://tyktech.atlassian.net/rest/api/3/issue/45154","key":"TT-13567","fields":{"summary":"[Docs] Review & update developer portal integration for streams","created":"2024-11-13T17:24:21.690+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/1","description":"The issue is open and ready for the assignee to start work on it.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/open.png","name":"Open","id":"1","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/2","id":2,"key":"new","colorName":"blue-gray","name":"To Do"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45153","self":"https://tyktech.atlassian.net/rest/api/3/issue/45153","key":"TD-3480","fields":{"summary":"FiscalNote ","created":"2024-11-13T17:23:33.246+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/3","description":"This issue is being actively worked on at the moment by the assignee.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/inprogress.png","name":"In Progress","id":"3","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/4","id":4,"key":"indeterminate","colorName":"yellow","name":"In Progress"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45151","self":"https://tyktech.atlassian.net/rest/api/3/issue/45151","key":"TT-13566","fields":{"summary":"Make upstream auth oauth password client secret not required in oas schema","created":"2024-11-13T15:03:48.591+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/10037","description":"","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/generic.png","name":"In Dev","id":"10037","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/4","id":4,"key":"indeterminate","colorName":"yellow","name":"In Progress"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45149","self":"https://tyktech.atlassian.net/rest/api/3/issue/45149","key":"TD-3479","fields":{"summary":"Liip/SBB Limits","created":"2024-11-13T13:43:55.347+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/3","description":"This issue is being actively worked on at the moment by the assignee.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/inprogress.png","name":"In Progress","id":"3","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/4","id":4,"key":"indeterminate","colorName":"yellow","name":"In Progress"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45148","self":"https://tyktech.atlassian.net/rest/api/3/issue/45148","key":"TT-13565","fields":{"summary":"Disable \"Hybrid data plane configuration\" option inside Type drop down if Control Plane is not deployed","created":"2024-11-13T13:15:05.346+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/1","description":"The issue is open and ready for the assignee to start work on it.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/open.png","name":"Open","id":"1","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/2","id":2,"key":"new","colorName":"blue-gray","name":"To Do"}}}}]}
`
