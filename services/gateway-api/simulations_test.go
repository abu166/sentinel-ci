package main

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulationRevisionIsValidHexSHA(t *testing.T) {
	for _, kind := range []string{"nil-pointer", "off-by-one", "data-race", "key-error", "context-timeout", "deadlock", "sql-injection"} {
		t.Run(kind, func(t *testing.T) {
			job, e := simulationJob(kind)
			if e != nil || !validSHA(job.CommitSHA) || len(job.CommitSHA) != 40 || !validID(job.PipelineID) {
				t.Fatal(job.CommitSHA, e)
			}
		})
	}
}
func TestSimulationDAGCommandsComplete(t *testing.T) {
	for _, spec := range simulations {
		j, e := simulationJob(spec.ID)
		if e != nil || validateSteps(j.Steps) != nil || len(j.Files) < 2 || j.CommitMessage == "" {
			t.Fatal(spec.ID, e)
		}
		if j.Steps[2].Name != "Unit Test Suite" {
			t.Fatal(j.Steps)
		}
	}
}
func TestSimulationGoSourcesParse(t *testing.T) {
	for _, kind := range []string{"nil-pointer", "data-race", "context-timeout", "deadlock"} {
		j, _ := simulationJob(kind)
		for name, source := range j.Files {
			if strings.HasSuffix(name, ".go") {
				if _, e := parser.ParseFile(token.NewFileSet(), name, source, parser.AllErrors); e != nil {
					t.Fatal(kind, name, e)
				}
			}
		}
	}
}
func TestSimulationCatalogHasFivePublicChoices(t *testing.T) {
	w := httptest.NewRecorder()
	simulationCatalog(w, httptest.NewRequest("GET", "/api/v1/simulations", nil))
	var rows []Simulation
	if json.Unmarshal(w.Body.Bytes(), &rows) != nil || len(rows) != 5 {
		t.Fatal(w.Body.String())
	}
}
func TestUnknownSimulationRejected(t *testing.T) {
	if _, e := simulationJob("not-a-repair"); e == nil {
		t.Fatal("unknown scenario accepted")
	}
	j, e := simulationJob("")
	if e != nil || !strings.Contains(j.Branch, "off-by-one") {
		t.Fatal(j, e)
	}
}
func TestSimulationBundlesStayRelative(t *testing.T) {
	for _, spec := range simulations {
		j, _ := simulationJob(spec.ID)
		for name := range j.Files {
			if filepath.IsAbs(name) || strings.Contains(name, "..") {
				t.Fatal(name)
			}
		}
	}
}
