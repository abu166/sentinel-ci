package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateSteps(t *testing.T) {
	tests := []struct {
		name  string
		steps []Step
		ok    bool
	}{{"empty", nil, false}, {"valid", []Step{{Name: "test", Command: "go test ./..."}}, true}, {"dependency", []Step{{Name: "a", Command: "true"}, {Name: "b", Command: "true", DependsOn: []string{"a"}}}, true}, {"cycle", []Step{{Name: "a", Command: "true", DependsOn: []string{"b"}}, {Name: "b", Command: "true", DependsOn: []string{"a"}}}, false}, {"unknown", []Step{{Name: "a", Command: "true", DependsOn: []string{"missing"}}}, false}, {"duplicate", []Step{{Name: "a", Command: "true"}, {Name: "a", Command: "true"}}, false}, {"command", []Step{{Name: "a"}}, false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateSteps(tt.steps) == nil; got != tt.ok {
				t.Fatalf("got %v", got)
			}
		})
	}
}
func TestUUID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := uuid()
		if !validID(id) || seen[id] {
			t.Fatal(id)
		}
		seen[id] = true
	}
	for _, id := range []string{"", "abcdef", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"} {
		if validID(id) {
			t.Fatal(id)
		}
	}
}
func TestHandlersRejectInvalidInput(t *testing.T) {
	a := &App{testMode:true}
	for _, path := range []string{"/api/v1/pipelines/bad", "/api/v1/pipelines/bad/simulate-failure", "/api/v1/patches/bad/apply", "/ws/pipelines/bad/logs"} {
		t.Run(path, func(t *testing.T) {
			method := "GET"
			if path == "/api/v1/pipelines/bad/simulate-failure" || path == "/api/v1/patches/bad/apply" {
				method = "POST"
			}
			w := httptest.NewRecorder()
			a.routes().ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 400 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestRegisterValidation(t *testing.T) {
	a := &App{testMode:true}
	for _, body := range []string{`{}`, `{"name":"repo","repo_url":"file:///etc/passwd"}`, `{"name":"repo","repo_url":"https://","unexpected":1}`, `{`} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/repos", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatalf("status %d", w.Code)
		}
	}
}
func TestTriggerValidation(t *testing.T) {
	a := &App{testMode:true}
	for _, body := range []string{`{}`, `{"repo_id":"` + uuid() + `","steps":[]}`, `{"repo_id":"` + uuid() + `","steps":[{"name":"x","command":"true","depends_on":["x"]}]}`} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/pipelines/trigger", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatalf("status %d", w.Code)
		}
	}
}
func TestPreflight(t *testing.T) {
	w := httptest.NewRecorder()
	(&App{testMode:true}).routes().ServeHTTP(w, httptest.NewRequest("OPTIONS", "/api/v1/repos", nil))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}
func TestDecodeAndWrite(t *testing.T) {
	w := httptest.NewRecorder()
	write(w, 201, map[string]string{"name": "test"})
	var b map[string]string
	if json.Unmarshal(w.Body.Bytes(), &b) != nil || b["name"] != "test" {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	var x map[string]any
	if decode(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{`)), &x) {
		t.Fatal("accepted malformed body")
	}
}
