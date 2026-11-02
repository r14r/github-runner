package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func runFixture(id int64, status, conclusion string) workflowRun {
	return workflowRun{ID: id, Name: "CI", DisplayTitle: "Test run", Status: status, Conclusion: conclusion, HeadBranch: "main"}
}

func fixtureActionsAPI(runs []workflowRun, events *[]string) actionsAPI {
	byID := make(map[int64]workflowRun)
	for _, r := range runs {
		byID[r.ID] = r
	}
	return actionsAPI{
		list: func(repo, state string) ([]workflowRun, error) {
			*events = append(*events, "list "+repo)
			return runs, nil
		},
		get: func(repo string, id int64) (workflowRun, error) {
			*events = append(*events, fmt.Sprintf("get %d", id))
			r, exists := byID[id]
			if !exists {
				return r, fmt.Errorf("not found: %d", id)
			}
			return r, nil
		},
		cancel: func(repo string, id int64) error {
			*events = append(*events, fmt.Sprintf("cancel %d", id))
			r := byID[id]
			r.Status, r.Conclusion = "completed", "cancelled"
			byID[id] = r
			return nil
		},
		delete: func(repo string, id int64) error {
			*events = append(*events, fmt.Sprintf("delete %d", id))
			if byID[id].Status != "completed" {
				t := byID[id]
				return fmt.Errorf("unsafe deletion of %d with status %s", id, t.Status)
			}
			return nil
		},
		sleep: func(time.Duration) {},
	}
}

func TestParseActionsArgsAndFilters(t *testing.T) {
	for _, tc := range []struct {
		args            []string
		op, repo, state string
		yes, withCancel bool
	}{
		{[]string{"list", "--repo", "example-org/demo-app"}, "list", "example-org/demo-app", "all", false, false},
		{[]string{"list", "--state", "success", "--repository=example-org/demo-app"}, "list", "example-org/demo-app", "success", false, false},
		{[]string{"cancel", "--repository", "example-org/demo-app", "--state=in_progress", "--yes"}, "cancel", "example-org/demo-app", "in_progress", true, false},
		{[]string{"delete", "--with-cancel", "--yes", "--state", "all", "--repo=example-org/demo-app"}, "delete", "example-org/demo-app", "all", true, true},
	} {
		o, err := parseActionsArgs(tc.args)
		if err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if o.operation != tc.op || o.repo != tc.repo || o.state != tc.state || o.yes != tc.yes || o.withCancel != tc.withCancel {
			t.Errorf("%v: got %+v", tc.args, o)
		}
	}
}

func TestActionsRejectInvalidFlagsBeforeAnySideEffects(t *testing.T) {
	bad := [][]string{
		nil, {"unknown"}, {"list"}, {"delete", "--state", "failure"},
		{"list", "--repo", "invalid"}, {"list", "--repo", "example-org/demo", "--yes"},
		{"cancel", "--repo", "example-org/demo", "--yes"},
		{"delete", "--repo", "example-org/demo", "--yes"},
		{"delete", "--repo", "example-org/demo", "--state", "invalid"},
		{"delete", "--repo", "example-org/demo", "--state"},
		{"list", "--repo", "example-org/demo", "--with-cancel"},
		{"cancel", "--repo", "example-org/demo", "--with-cancel"},
		{"delete", "--repo", "example-org/demo", "--state", "all", "--state", "completed"},
		{"delete", "--repo", "example-org/demo", "--state=bogus", "--yes"},
		{"delete", "--repo", "example-org/demo", "--state", "all", "--bogus"},
	}
	for _, args := range bad {
		if _, err := parseActionsArgs(args); err == nil {
			t.Errorf("accepted invalid options: %q", args)
		}
	}
	called := false
	api := actionsAPI{list: func(string, string) ([]workflowRun, error) { called = true; return nil, nil }}
	if err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--yes"}, api, &bytes.Buffer{}); err == nil || called {
		t.Fatal("destructive request without state reached GitHub")
	}
}

func TestActionsStateMatchesGithubStatusesAndConclusions(t *testing.T) {
	for _, state := range actionsStates {
		if !validActionsState(state.Name) {
			t.Errorf("state %q missing from parser", state.Name)
		}
	}
	if validActionsState("destroyed") || validActionsState("") {
		t.Fatal("invalid state accepted")
	}
	failed := runFixture(1, "completed", "failure")
	active := runFixture(2, "in_progress", "")
	for _, tc := range []struct {
		run   workflowRun
		state string
		want  bool
	}{
		{failed, "all", true}, {failed, "failure", true},
		{failed, "completed", true}, {failed, "success", false},
		{active, "in_progress", true}, {active, "queued", false},
		{active, "all", true},
	} {
		if got := stateMatchesRun(tc.run, tc.state); got != tc.want {
			t.Errorf("stateMatchesRun(%+v, %q) = %t", tc.run, tc.state, got)
		}
	}
}

func TestActionsListPrintsAllMatchingRunsWithoutMutation(t *testing.T) {
	runs := []workflowRun{runFixture(11, "completed", "success"), runFixture(12, "completed", "failure"), runFixture(13, "in_progress", "")}
	events := []string{}
	api := fixtureActionsAPI(runs, &events)
	var output bytes.Buffer
	err := actionsCommand([]string{"list", "--repo", "example-org/demo", "--state", "failure"}, api, &output)
	if err != nil {
		t.Fatal(err)
	}
	s := output.String()
	for _, fragment := range []string{"Repository: example-org/demo", "State: failure", "Matching workflow runs: 1", "12", "failure", "WORKFLOW"} {
		if !strings.Contains(s, fragment) {
			t.Errorf("missing %q in %q", fragment, s)
		}
	}
	if strings.Contains(s, "11 ") || strings.Contains(s, "13 ") {
		t.Errorf("unmatched run included in output: %s", s)
	}
	if !reflect.DeepEqual(events, []string{"list example-org/demo"}) {
		t.Errorf("list must not change GitHub: %v", events)
	}
}

func TestActionsCancelDryRunAndExecution(t *testing.T) {
	runs := []workflowRun{runFixture(1, "in_progress", ""), runFixture(2, "queued", ""), runFixture(3, "completed", "success")}
	events := []string{}
	api := fixtureActionsAPI(runs, &events)
	var output bytes.Buffer
	if err := actionsCommand([]string{"cancel", "--repo", "example-org/demo", "--state", "all"}, api, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "WOULD CANCEL") != 2 || strings.Contains(output.String(), "CANCELLED") {
		t.Errorf("dry run output: %s", output.String())
	}
	if len(events) != 1 {
		t.Fatalf("dry-run made API mutations: %v", events)
	}
	events = nil
	output.Reset()
	if err := actionsCommand([]string{"cancel", "--repo", "example-org/demo", "--state", "all", "--yes"}, api, &output); err != nil {
		t.Fatal(err)
	}
	want := []string{"list example-org/demo", "get 1", "cancel 1", "get 2", "cancel 2"}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("actual API calls %v; want %v", events, want)
	}
	if strings.Count(output.String(), "CANCELLED") != 2 {
		t.Errorf("cancel results: %s", output.String())
	}
}

func TestActionsDeleteDryRunSkipsActive(t *testing.T) {
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{
		runFixture(1, "completed", "failure"), runFixture(2, "in_progress", ""),
	}, &events)
	var output bytes.Buffer
	if err := actionsCommand([]string{"delete", "--repo", "example-org/demo"}, api, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "WOULD DELETE 1") || strings.Contains(output.String(), "WOULD DELETE 2") {
		t.Errorf("unexpected dry-run: %s", output.String())
	}
	if len(events) != 1 {
		t.Fatalf("dry-run was destructive: %v", events)
	}
}

func TestActionsDeleteOnlyMatchingCompletedRuns(t *testing.T) {
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{
		runFixture(1, "completed", "failure"),
		runFixture(2, "completed", "success"),
		runFixture(3, "in_progress", ""),
	}, &events)
	var output bytes.Buffer
	if err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--state", "failure", "--yes"}, api, &output); err != nil {
		t.Fatal(err)
	}
	want := []string{"list example-org/demo", "get 1", "delete 1"}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("API calls: %v; want %v", events, want)
	}
	if !strings.Contains(output.String(), "DELETED 1") {
		t.Fatal(output.String())
	}
}

func TestDeleteWithCancelWaitsForCompletionThenDeletes(t *testing.T) {
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{
		runFixture(1, "in_progress", ""), runFixture(2, "completed", "failure"),
	}, &events)
	var output bytes.Buffer
	if err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--state", "all", "--with-cancel", "--yes"}, api, &output); err != nil {
		t.Fatal(err)
	}
	want := []string{"list example-org/demo", "get 1", "cancel 1", "get 1", "delete 1", "get 2", "delete 2"}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("API calls: %v; want %v", events, want)
	}
	if strings.Count(output.String(), "DELETED") != 2 {
		t.Error(output.String())
	}
}

func TestDeleteWithCancelDryRunDoesNotCancel(t *testing.T) {
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{
		runFixture(1, "in_progress", ""), runFixture(2, "completed", "failure"),
	}, &events)
	var out bytes.Buffer
	if err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--state", "all", "--with-cancel"}, api, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "WOULD CANCEL THEN DELETE 1") || !strings.Contains(out.String(), "WOULD DELETE 2") {
		t.Errorf("dry-run: %s", out.String())
	}
	if !reflect.DeepEqual(events, []string{"list example-org/demo"}) {
		t.Fatal(events)
	}
}

func TestDeleteCancelTimeoutPreservesRun(t *testing.T) {
	r := runFixture(1, "in_progress", "")
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{r}, &events)
	api.get = func(string, int64) (workflowRun, error) { events = append(events, "get"); return r, nil }
	api.sleep = func(time.Duration) {}
	var out bytes.Buffer
	err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--state", "in_progress", "--with-cancel", "--yes"}, api, &out)
	if err == nil || !strings.Contains(err.Error(), "not deleted") {
		t.Errorf("expected timeout: %v", err)
	}
	for _, event := range events {
		if strings.HasPrefix(event, "delete ") {
			t.Fatalf("deleted running workflow run: %v", events)
		}
	}
}

func TestDeletionRechecksStatusAndSkipsChangedConclusion(t *testing.T) {
	initial := runFixture(1, "completed", "failure")
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{initial}, &events)
	api.get = func(string, int64) (workflowRun, error) {
		events = append(events, "get")
		return runFixture(1, "completed", "success"), nil
	}
	if err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--state", "failure", "--yes"}, api, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"list example-org/demo", "get"}) {
		t.Errorf("conclusion changed: deleted anyway %v", events)
	}
}

func TestActionsListFailureDoesNotMutate(t *testing.T) {
	api := actionsAPI{list: func(string, string) ([]workflowRun, error) { return nil, errors.New("GitHub unavailable") }}
	err := actionsCommand([]string{"delete", "--repo", "example-org/demo", "--state", "all", "--yes"}, api, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "GitHub unavailable") {
		t.Fatal(err)
	}
}

func TestActionsHelpWorksWithoutAssetsAndShowsAllPossibleStates(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", t.TempDir()+"/missing")
	for _, args := range [][]string{
		{"actions", "help"},
		{"actions", "list", "--help"},
		{"actions", "cancel", "help"},
		{"actions", "delete", "--help"},
		{"help", "actions", "delete"},
	} {
		out, err := captureCLI(t, args...)
		if err != nil {
			t.Errorf("help %v: %v", args, err)
			continue
		}
		for _, expected := range []string{"AVAILABLE STATES:", "success", "failure", "cancelled", "timed_out", "in_progress", "queued", "completed", "EXAMPLES:"} {
			if !strings.Contains(out, expected) {
				t.Errorf("help %v missing %q", args, expected)
			}
		}
	}
}

func TestFetchAllWorkflowRunsPaginatesBeforeMutation(t *testing.T) {
	var endpoints []string
	makePage := func(ids []workflowRun, count int) []byte {
		payload, err := json.Marshal(struct {
			TotalCount   int           `json:"total_count"`
			WorkflowRuns []workflowRun `json:"workflow_runs"`
		}{TotalCount: count, WorkflowRuns: ids})
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	page1 := make([]workflowRun, 100)
	for i := range page1 {
		page1[i] = runFixture(int64(i+1), "completed", "failure")
	}
	page2 := []workflowRun{runFixture(101, "completed", "success")}
	runs, err := fetchAllWorkflowRuns("example-org/demo", func(endpoint string) ([]byte, error) {
		endpoints = append(endpoints, endpoint)
		if strings.HasSuffix(endpoint, "&page=1") {
			return makePage(page1, 101), nil
		}
		if strings.HasSuffix(endpoint, "&page=2") {
			return makePage(page2, 101), nil
		}
		return nil, fmt.Errorf("unexpected page %q", endpoint)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 101 || len(endpoints) != 2 || runs[100].ID != 101 {
		t.Fatalf("pagination runs=%d endpoints=%v", len(runs), endpoints)
	}
}

func TestFetchAllWorkflowRunsRejectsIncompleteOrDuplicatedPages(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		errorMessage string
	}{
		{"incomplete", []byte(`{"total_count":7,"workflow_runs":[{"id":1,"status":"completed"}]}`), "partial result"},
		{"badJSON", []byte(`{ broken`), "invalid"},
		{"duplicate", []byte(`{"total_count":2,"workflow_runs":[{"id":1},{"id":1}]}`), "duplicate"},
		{"invalidID", []byte(`{"total_count":1,"workflow_runs":[{"id":0}]}`), "invalid run"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fetchAllWorkflowRuns("example-org/demo", func(string) ([]byte, error) { return tc.body, nil })
			if err == nil || !strings.Contains(err.Error(), tc.errorMessage) {
				t.Fatalf("%s expected failure: %v", tc.name, err)
			}
		})
	}
}

func TestCancelStatusChangedBeforeExecutionIsSkipped(t *testing.T) {
	run := runFixture(1, "in_progress", "")
	events := []string{}
	api := fixtureActionsAPI([]workflowRun{run}, &events)
	api.get = func(string, int64) (workflowRun, error) {
		events = append(events, "get")
		return runFixture(1, "queued", ""), nil
	}
	err := actionsCommand([]string{"cancel", "--repo", "example-org/demo", "--state", "in_progress", "--yes"}, api, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"list example-org/demo", "get"}) {
		t.Fatalf("cancelled a changed state: %v", events)
	}
}

func TestFilteredSearchUsesServerSideStatus(t *testing.T) {
	var endpoints []string
	jsonPage:=[]byte(`{"total_count":2,"workflow_runs":[{"id":100,"status":"completed","conclusion":"failure"},{"id":101,"status":"completed","conclusion":"failure"}]}`)
	runs, err := fetchWorkflowRunSnapshot("example-org/demo","failure",func(endpoint string)([]byte,error){
		endpoints=append(endpoints,endpoint)
		return jsonPage,nil
	},time.Date(2026,11,2,0,0,0,0,time.UTC))
	if err!=nil{t.Fatal(err)}
	if len(runs)!=2||len(endpoints)!=1||!strings.Contains(endpoints[0],"status=failure"){
		t.Errorf("expected one server-side failure filter, got %v; runs=%v",endpoints,runs)
	}
}

func TestFilteredSearchPartitionsOverThousandRunGitHubCap(t *testing.T) {
	var endpoints []string
	now:=time.Date(2026,11,2,10,0,0,0,time.UTC)
	runs,err:=fetchWorkflowRunSnapshot("example-org/demo","failure",func(endpoint string)([]byte,error){
		endpoints=append(endpoints,endpoint)
		if !strings.Contains(endpoint,"status=failure"){return nil,fmt.Errorf("missing status filter: %s",endpoint)}
		if !strings.Contains(endpoint,"created="){return []byte(`{"total_count":1000,"workflow_runs":[{"id":1,"status":"completed","conclusion":"failure"}]}`),nil}
		if strings.Contains(endpoint,"2018-01-01"){return []byte(`{"total_count":1,"workflow_runs":[{"id":1,"status":"completed","conclusion":"failure"}]}`),nil}
		return []byte(`{"total_count":1,"workflow_runs":[{"id":2,"status":"completed","conclusion":"failure"}]}`),nil
	},now)
	if err!=nil{t.Fatal(err)}
	if len(runs)!=2||len(endpoints)!=3{t.Errorf("expected 2 shards and 2 runs, got %d and %v",len(runs),endpoints)}
	for _,endpoint:=range endpoints[1:]{
		if !strings.Contains(endpoint,"created="){t.Errorf("missing date shard: %s",endpoint)}
	}
}

func TestFilteredSearchRejectsUnpartitionableResults(t *testing.T) {
	now:=time.Date(2026,11,2,10,0,0,0,time.UTC)
	_,err:=fetchWorkflowRunWindow("example-org/demo","failure",now,now,func(string)([]byte,error){
		return []byte(`{"total_count":1000,"workflow_runs":[]}`),nil
	},0)
	if err==nil||!strings.Contains(err.Error(),"cannot enumerate all safely"){t.Fatalf("expected failure for >1000 same-second records: %v",err)}
}

func TestFilteredSearchRejectsWrongStateFromGitHub(t *testing.T) {
	_,err:=fetchWorkflowRunSnapshot("example-org/demo","failure",func(string)([]byte,error){
		return []byte(`{"total_count":1,"workflow_runs":[{"id":42,"status":"completed","conclusion":"success"}]}`),nil
	},time.Now())
	if err==nil||!strings.Contains(err.Error(),"outside --state"){t.Fatalf("unexpected response accepted: %v",err)}
}

func TestFilteredBulkDeletePassesStateToAPI(t *testing.T) {
	var selectedState string
	var ops []string
	api:=actionsAPI{
		list:func(repo,state string)([]workflowRun,error){
			selectedState=state
			return []workflowRun{runFixture(1,"completed","failure")},nil
		},
		get:func(string,int64)(workflowRun,error){ops=append(ops,"get");return runFixture(1,"completed","failure"),nil},
		delete:func(string,int64)error{ops=append(ops,"delete");return nil},
	}
	if err:=actionsCommand([]string{"delete","--repo","example-org/demo","--state","failure","--yes"},api,&bytes.Buffer{});err!=nil{t.Fatal(err)}
	if selectedState!="failure"||!reflect.DeepEqual(ops,[]string{"get","delete"}){t.Errorf("state=%s ops=%v",selectedState,ops)}
}
