package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type APIServer struct {
	users  map[int]User
	mu     sync.RWMutex
	nextID int
}

func NewAPIServer() *APIServer {
	return &APIServer{
		users:  make(map[int]User),
		nextID: 1,
	}
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *APIServer) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Simulate some processing time
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func (s *APIServer) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	user.ID = s.nextID
	s.nextID++
	s.users[user.ID] = user
	s.mu.Unlock()

	// Simulate some processing time
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (s *APIServer) handleGetData(w http.ResponseWriter, r *http.Request) {
	// Simulate variable processing time
	delay := rand.Intn(200) + 50
	time.Sleep(time.Duration(delay) * time.Millisecond)

	// Sometimes return errors to test error handling
	if rand.Float32() < 0.05 { // 5% error rate
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"data": map[string]int{
			"value1": rand.Intn(1000),
			"value2": rand.Intn(1000),
			"value3": rand.Intn(1000),
		},
		"processingTime": delay,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *APIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Performance Test App</title>
</head>
<body>
    <h1>Performance Test Application</h1>
    <form id="loginForm">
        <input type="email" name="email" placeholder="Email" required>
        <input type="password" name="password" placeholder="Password" required>
        <button type="submit">Login</button>
    </form>
    <div id="welcome" style="display:none;">
        <h1>Welcome!</h1>
    </div>
    <script>
        document.getElementById('loginForm').addEventListener('submit', function(e) {
            e.preventDefault();
            document.getElementById('loginForm').style.display = 'none';
            document.getElementById('welcome').style.display = 'block';
        });
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func main() {
	server := NewAPIServer()

	// Add some initial data
	server.users[1] = User{ID: 1, Name: "John Doe", Email: "john@example.com"}
	server.users[2] = User{ID: 2, Name: "Jane Smith", Email: "jane@example.com"}
	server.nextID = 3

	mux := http.NewServeMux()
	
	// API routes
	mux.HandleFunc("/api/health", server.handleHealth)
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			server.handleGetUsers(w, r)
		case http.MethodPost:
			server.handleCreateUser(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/data", server.handleGetData)
	mux.HandleFunc("/", server.handleIndex)

	// Middleware for logging
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mux.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})

	log.Println("Starting web server on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}