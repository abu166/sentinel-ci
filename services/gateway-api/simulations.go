package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
)

type Simulation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Language   string `json:"language"`
	Message    string `json:"commit_message"`
}

var simulations = []Simulation{
	{"nil-pointer", "Go Nil-Pointer Dereference", "payment-gateway", "go", "fix(payments): validate optional settlement request"},
	{"off-by-one", "Python Off-by-One Slice Boundary", "inventory-worker", "python", "fix(inventory): bound final stock batch index"},
	{"data-race", "Go Concurrent Map Write / Data Race", "order-matching-engine", "go", "fix(matching): serialize order-book counter writes"},
	{"key-error", "Python Unhandled KeyError in Async Worker", "telemetry-ingestor", "python", "fix(telemetry): handle events without an identifier"},
	{"context-timeout", "Go Context Timeout / Goroutine Leak", "auth-proxy", "go", "fix(auth): honor request cancellation while awaiting tokens"},
}

func simulationCatalog(w http.ResponseWriter, r *http.Request) { write(w, 200, simulations) }
func simulationJob(kind string) (Job, error) {
	if kind == "" {
		kind = "off-by-one"
	}
	var spec *Simulation
	for i := range simulations {
		if simulations[i].ID == kind {
			spec = &simulations[i]
			break
		}
	}
	if spec == nil && kind == "deadlock" {
		value := simulations[4]
		value.ID = "deadlock"
		value.Name = "Go Deadlock / Ignored Cancellation"
		spec = &value
	}
	if spec == nil && kind == "sql-injection" {
		value := Simulation{ID: "sql-injection", Name: "SQL Injection", Repository: "auth-proxy", Language: "python", Message: "fix(auth): parameterize account lookup"}
		spec = &value
	}
	if spec == nil {
		return Job{}, errors.New("unsupported scenario; choose nil-pointer, off-by-one, data-race, key-error or context-timeout")
	}
	files := map[string]string{}
	command := "python3 -m unittest discover -v"
	lint := "python3 -m compileall -q ."
	checkout := "sha256sum *.py"
	switch kind {
	case "sql-injection":
		files["accounts.py"] = "import sqlite3\ndef lookup_user(db, name):\n    return db.execute(\"SELECT id FROM users WHERE name = '\" + name + \"'\").fetchall()\n"
		files["test_accounts.py"] = "import sqlite3\nimport unittest\nfrom accounts import lookup_user\nclass AccountTests(unittest.TestCase):\n    def setUp(self):\n        self.db=sqlite3.connect(':memory:')\n        self.db.execute('CREATE TABLE users(id INTEGER, name TEXT)')\n        self.db.executemany('INSERT INTO users VALUES (?, ?)',[(1,'alice'),(2,'bob'),(3,\"O'Reilly\")])\n    def tearDown(self): self.db.close()\n    def test_user(self): self.assertEqual(lookup_user(self.db,'alice'),[(1,)])\n    def test_injection(self): self.assertEqual(lookup_user(self.db,\"' OR 1=1 --\"),[], 'SQL injection must not return user records')\n    def test_quote(self): self.assertEqual(lookup_user(self.db,\"O'Reilly\"),[(3,)])\n"
	case "off-by-one":
		files["inventory.py"] = "def last_item(items):\n    return items[len(items)]\n"
		files["test_inventory.py"] = "import unittest\nfrom inventory import last_item\nclass InventoryTests(unittest.TestCase):\n    def test_last(self): self.assertEqual(last_item([1, 2, 3]), 3)\n    def test_single(self): self.assertEqual(last_item([8]), 8)\n"
	case "key-error":
		files["telemetry.py"] = "import asyncio\ndef event_id(event):\n    return event[\"id\"]\nasync def ingest(event):\n    await asyncio.sleep(0)\n    return event_id(event)\n"
		files["test_telemetry.py"] = "import asyncio\nimport unittest\nfrom telemetry import ingest\nclass TelemetryTests(unittest.TestCase):\n    def test_missing(self): self.assertIsNone(asyncio.run(ingest({})))\n    def test_present(self): self.assertEqual(asyncio.run(ingest({'id': 'evt-42'})), 'evt-42')\n"
	case "nil-pointer":
		files["payment.go"] = "package repair\ntype Payment struct { Amount int }\nfunc Amount(p *Payment) int {\n return p.Amount\n}\n"
		files["payment_test.go"] = "package repair\nimport \"testing\"\nfunc TestNilPayment(t *testing.T) { if Amount(nil)!=0 {t.Fatal(\"nil payment must return zero\")} }\nfunc TestPayment(t *testing.T) { if Amount(&Payment{Amount:42})!=42 {t.Fatal(\"amount changed\")} }\n"
	case "data-race":
		files["orders.go"] = "package repair\nimport \"sync\"\nvar mu sync.Mutex\nvar values = map[string]int{}\nfunc increment() {\n values[\"orders\"]++\n}\n"
		files["orders_test.go"] = "package repair\nimport (\"testing\"; \"sync\")\nfunc TestConcurrentOrders(t *testing.T) { var wg sync.WaitGroup; for i:=0;i<8;i++ { wg.Add(1); go func(){defer wg.Done();for j:=0;j<100;j++{increment()}}() }; wg.Wait(); if values[\"orders\"]!=800 {t.Fatalf(\"lost orders: %d\",values[\"orders\"])} }\n"
		command = "go test -race -timeout 30s ./..."
	case "context-timeout", "deadlock":
		files["proxy.go"] = "package repair\nimport \"context\"\nfunc Await(ctx context.Context, input <-chan int) (int, error) {\n select {\n case value := <-input:\n  return value, nil\n }\n}\n"
		files["proxy_test.go"] = "package repair\nimport (\"context\"; \"testing\"; \"time\"; \"errors\")\nfunc TestDeadline(t *testing.T) {ctx,cancel:=context.WithTimeout(context.Background(),20*time.Millisecond);defer cancel();_,err:=Await(ctx,make(chan int));if !errors.Is(err,context.DeadlineExceeded){t.Fatalf(\"deadline not propagated: %v\",err)}}\nfunc TestReady(t *testing.T) {ch:=make(chan int,1);ch<-42;v,err:=Await(context.Background(),ch);if v!=42||err!=nil{t.Fatal(v,err)}}\n"
		command = "go test -timeout 2s ./..."
	}
	if spec.Language == "go" {
		files["go.mod"] = "module sentinel/repair\n\ngo 1.23\n"
		if kind != "data-race" && kind != "context-timeout" && kind != "deadlock" {
			command = "go test -timeout 30s ./..."
		}
		lint = "go vet ./..."
		checkout = "sha256sum go.mod *.go"
	}
	sum := sha256.Sum256([]byte(kind + spec.Message))
	sha := hex.EncodeToString(sum[:20])
	return Job{PipelineID: uuid(), CommitSHA: sha, CommitMessage: spec.Message, Branch: "reproduce/" + kind, Files: files, Steps: []Step{
		{Name: "Checkout", Stage: "01-checkout", Command: checkout, DependsOn: []string{}},
		{Name: "Lint & Static Check", Stage: "02-lint", Command: lint, DependsOn: []string{"Checkout"}},
		{Name: "Unit Test Suite", Stage: "03-test", Command: command, DependsOn: []string{"Lint & Static Check"}},
	}}, nil
}
func (a *App) simulate(w http.ResponseWriter, r *http.Request) {
	if !validID(r.PathValue("id")) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	var input struct {
		Scenario string `json:"scenario"`
	}
	if r.ContentLength != 0 {
		if !decode(w, r, &input) {
			return
		}
	}
	job, err := simulationJob(input.Scenario)
	if err != nil {
		fail(w, 400, err)
		return
	}
	var repo string
	if err = a.db.QueryRow(r.Context(), "SELECT repo_id::text FROM pipelines WHERE id=$1", r.PathValue("id")).Scan(&repo); err != nil {
		fail(w, 404, errors.New("pipeline not found"))
		return
	}
	// Prefer the catalog's matching repository while preserving compatibility with an existing registry.
	for _, spec := range simulations {
		if spec.ID == input.Scenario {
			var match string
			if a.db.QueryRow(r.Context(), "SELECT id::text FROM repositories WHERE name=$1", spec.Repository).Scan(&match) == nil {
				repo = match
			}
			break
		}
	}
	if err = a.createJob(r.Context(), repo, &job); err != nil {
		fail(w, 503, err)
		return
	}
	write(w, 202, map[string]string{"id": job.PipelineID, "pipeline_id": job.PipelineID, "status": "PENDING"})
}
