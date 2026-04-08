package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"
)

// User represents a user object stored in Redis
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

// Global variables for Redis client and context
var (
	ctx = context.Background() // Context for Redis operations
	rdb *redis.Client          // Redis client
)

func main() {
	// Initialize Redis client pointing to the redis service on port 6379
	rdb = redis.NewClient(&redis.Options{
		Addr: "redis:6379",
		DB:   0,
	})

	// Verify Redis is reachable before starting the server
	_, err := rdb.Ping(ctx).Result() 
	if err != nil {
		log.Fatal("failed to connect to redis:", err)
	}

	// Register route handlers: /users for collection, /users/{id} for single resource
	http.HandleFunc("/users", usersHandler) // register the route, which will dispatch to usersHandler for both POST and GET methods
	http.HandleFunc("/users/", userByIDHandler)

	fmt.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil)) // Start the HTTP server on port 8080 and log any fatal errors
}

// usersHandler dispatches POST (create) and GET (list) requests on /users
func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createUser(w, r)
	case http.MethodGet:
		listUsers(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// userByIDHandler extracts the user ID from the URL path and dispatches
// GET (fetch), PUT (update), and DELETE requests on /users/{id}
func userByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/users/")
	if id == "" {
		http.Error(w, "missing user id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getUser(w, r, id)
	case http.MethodPut:
		updateUser(w, r, id)
	case http.MethodDelete:
		deleteUser(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// createUser decodes a User from the request body, checks for duplicate IDs,
// stores the user JSON in Redis under "user:{id}", and adds the ID to the "users" set
func createUser(w http.ResponseWriter, r *http.Request) {
	// Decode request body into a User struct
	var user User
	// json.NewDecoder(r.Body) reads bytes from the HTTP request body.
	// .Decode(&user) tries to parse that JSON and fill in user's fields.(because &user lets it modify the struct).
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if user.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	// Reject the request if a user with the same ID already exists
	key := "user:" + user.ID
	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	if exists == 1 {
		http.Error(w, "user already exists", http.StatusConflict)
		return
	}

	// Serialize the user and persist it in Redis
	// json.Marshal(user) converts the user struct into a JSON byte slice. 
	data, err := json.Marshal(user) 
	if err != nil {
		http.Error(w, "json error", http.StatusInternalServerError)
		return
	}
	// Store the data JSON string in Redis under the key "user:{id}"
	if err := rdb.Set(ctx, key, data, 0).Err(); err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	// Track the new user ID in a Redis set for easy enumeration
	if err := rdb.SAdd(ctx, "users", user.ID).Err(); err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}

	// Respond with the created user as JSON and a 201 Created status code	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(user); err != nil {
	http.Error(w, "response error", http.StatusInternalServerError)
	return
	}
	// Converts the Go user struct into JSON and writes it into the response body 
	// via w (the ResponseWriter).
}

// getUser fetches a single user by ID from Redis and writes it as JSON
func getUser(w http.ResponseWriter, r *http.Request, id string) {
	key := "user:" + id

	// Retrieve raw JSON from Redis; return 404 if the key doesn't exist
	data, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(data)); err != nil {
	http.Error(w, "response error", http.StatusInternalServerError)
	return
	}
}

// updateUser replaces an existing user's data; returns 404 if the user doesn't exist
func updateUser(w http.ResponseWriter, r *http.Request, id string) {
	key := "user:" + id

	// Ensure the user exists before attempting an update
	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	if exists == 0 {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Decode the updated fields from the request body
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Force the ID to match the URL parameter, ignoring any body value
	user.ID = id

	// Serialize and overwrite the existing record in Redis
	data, err := json.Marshal(user)
	if err != nil {
		http.Error(w, "json error", http.StatusInternalServerError)
		return
	}

	// Update the user's data record in Redis with the new JSON string. The key remains the same, so this effectively replaces the old data.
	if err := rdb.Set(ctx, key, data, 0).Err(); err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	// Re-add the ID to the set (no-op if already present)
	if err := rdb.SAdd(ctx, "users", id).Err(); err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(data)); err != nil {
	http.Error(w, "response error", http.StatusInternalServerError)
	return
	}
}

// deleteUser removes the user's key from Redis and its ID from the "users" set
func deleteUser(w http.ResponseWriter, r *http.Request, id string) {
	key := "user:" + id

	// Delete the user's data record
	if err := rdb.Del(ctx, key).Err(); err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	// Remove the ID from the tracking set
	if err := rdb.SRem(ctx, "users", id).Err(); err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listUsers fetches all user IDs from the "users" set, retrieves each user's
// data from Redis, and returns them as a JSON array
func listUsers(w http.ResponseWriter, r *http.Request) {
	// Get all tracked user IDs from the set
	ids, err := rdb.SMembers(ctx, "users").Result()
	if err != nil {
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}

	// Fetch and deserialize each user, skipping any that fail
	users := []User{} // Initialize an empty slice to hold the user objects
	for _, id := range ids { 
		data, err := rdb.Get(ctx, "user:"+id).Result()
		if err != nil {
			continue
		}

		var user User
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			continue
		}
		users = append(users, user)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(users); err != nil {
	http.Error(w, "response error", http.StatusInternalServerError)
	return
	}
}
