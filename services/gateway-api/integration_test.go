package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func mockApp(t *testing.T) (*App, pgxmock.PgxPoolIface) {
	t.Helper()
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return &App{db: db, testMode: true}, db
}
func TestRepositoryQueries(t *testing.T) {
	for _, tc := range []struct{ name, path, query string }{{"repos", "/api/v1/repos", "SELECT r.id,r.name"}, {"pipelines", "/api/v1/pipelines", "SELECT p.\\*, r.name"}, {"patches", "/api/v1/patches", "SELECT \\* FROM healing_patches"}} {
		t.Run(tc.name, func(t *testing.T) {
			a, db := mockApp(t)
			e := db.ExpectQuery(tc.query)
			if tc.name == "pipelines" {
				e.WithArgs("", "")
			}
			if tc.name == "patches" {
				e.WithArgs("")
			}
			e.WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("example"))
			w := httptest.NewRecorder()
			a.routes().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != 200 || !strings.Contains(w.Body.String(), "example") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRepositoryFailure(t *testing.T) {
	a, db := mockApp(t)
	db.ExpectQuery("SELECT r.id,r.name").WillReturnError(errors.New("connection unavailable"))
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/repos", nil))
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
}
func TestPipelineDetail(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectQuery("SELECT p.\\*,r.name").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"id", "status"}).AddRow(id, "FAILED"))
	db.ExpectQuery("SELECT \\* FROM pipeline_steps").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).AddRow(uuid(), "tests"))
	db.ExpectQuery("SELECT \\* FROM healing_patches").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"id"}))
	db.ExpectQuery("SELECT l.\\*").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"content"}).AddRow("failed test"))
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/pipelines/"+id, nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var detail map[string]any
	if json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail["steps"].([]any)) != 1 {
		t.Fatal(w.Body.String())
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestApplyRejectsUnverified(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectQuery("SELECT pipeline_id::text,file_path").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"pipeline_id", "file_path", "patched_code", "verification_status", "selected_candidate_id"}).AddRow(uuid(), "app.py", "fixed", "REJECTED", ""))
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/patches/"+id+"/apply", nil))
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestWebSocketConcurrentSubscribers(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	a := &App{redis: rc, testMode: true}
	server := httptest.NewServer(a.routes())
	defer server.Close()
	id := uuid()
	const clients = 16
	connections := make([]*websocket.Conn, 0, clients)
	for i := 0; i < clients; i++ {
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/pipelines/"+id+"/logs", nil)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, c)
		defer c.Close()
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		n, err := rc.PubSubNumSub(context.Background(), "pipeline:"+id+":logs").Result()
		if err != nil {
			t.Fatal(err)
		}
		if n["pipeline:"+id+":logs"] == clients {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscribers not ready")
		}
		time.Sleep(time.Millisecond)
	}
	payload := `{"content":"live build output","stream":"stdout"}`
	if err := rc.Publish(context.Background(), "pipeline:"+id+":logs", payload).Err(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, c := range connections {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			c.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, b, err := c.ReadMessage()
			if err != nil || string(b) != payload {
				t.Errorf("message %s error %v", b, err)
			}
		}(c)
	}
	wg.Wait()
}
func TestRepositoryUUIDEncoding(t *testing.T) {
	a, db := mockApp(t)
	id := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	db.ExpectQuery("SELECT").WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(id))
	rows, err := a.query(context.Background(), "SELECT id FROM repositories")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["id"] != "01020304-0506-0708-090a-0b0c0d0e0f10" {
		t.Fatal(rows)
	}
}
