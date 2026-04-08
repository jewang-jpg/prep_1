package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setup initializes a miniredis instance and configures the global Redis client to use it for testing. It also ensures that the miniredis server is closed after the test completes.
func setup(t *testing.T) {   // t is the testing object passed to each test function, which allows us to manage test lifecycle and report errors.
	mr, _ := miniredis.Run() // Start a new miniredis server, It returns a server instance and an error (which we ignore here for simplicity).
	rdb = redis.NewClient(&redis.Options{Addr: mr.Addr()}) // Create a new Redis client that connects to the miniredis server using its address. 
	ctx = context.Background()
	t.Cleanup(func() { mr.Close() })
}

// TestCreateAndGetUser verifies that a user can be created via POST /users
// and then retrieved via GET /users/{id}.
func TestCreateAndGetUser(t *testing.T) {
	setup(t)

	// POST /users — should return 201 Created
	body, _ := json.Marshal(User{ID: "1", Name: "Alice", Email: "alice@example.com", Age: 30}) 
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body)) // httptest.NewRequest wants the body as an io.Reader, so we wrap the JSON bytes in a bytes.NewReader.
	w := httptest.NewRecorder() // Creates a fake response writer.
	usersHandler(w, req) // Directly call the usersHandler function with our fake request and response writer. This simulates handling an HTTP request without needing to start an actual server.

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// GET /users/1 — should return 200 OK with the same user
	req2 := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	w2 := httptest.NewRecorder()
	getUser(w2, req2, "1")

	if w2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w2.Code)
	}
}

// TestGetUser_NotFound verifies that fetching a non-existent user returns 404.
func TestGetUser_NotFound(t *testing.T) {
	setup(t)

	req := httptest.NewRequest(http.MethodGet, "/users/999", nil)
	w := httptest.NewRecorder()
	getUser(w, req, "999")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestDeleteUser verifies that deleting an existing user returns 204 No Content.
func TestDeleteUser(t *testing.T) {
	setup(t)

	// First create a user so there is something to delete
	body, _ := json.Marshal(User{ID: "2", Name: "Bob"})
	usersHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body)))

	// DELETE /users/2 — should return 204 No Content
	req := httptest.NewRequest(http.MethodDelete, "/users/2", nil)
	w := httptest.NewRecorder()
	deleteUser(w, req, "2")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}
