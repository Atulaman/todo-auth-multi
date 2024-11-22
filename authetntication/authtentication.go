package authetntication

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
	"unicode/utf8"

	_ "github.com/lib/pq"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var db *sql.DB

func SetDB(database *sql.DB) {
	db = database
}
func Register(w http.ResponseWriter, r *http.Request) {
	var user User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user.Username == "" || user.Password == "" || utf8.RuneCountInString(user.Username) > 20 || utf8.RuneCountInString(user.Password) > 20 || utf8.RuneCountInString(user.Password) < 8 || utf8.RuneCountInString(user.Username) < 8 {
		http.Error(w, "Missing/Invalid username or password", http.StatusBadRequest)
		return
	}
	_, err = db.Exec("INSERT INTO auth (username, password) VALUES ($1, $2)", user.Username, user.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Registration successful"})
}
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func Login(w http.ResponseWriter, r *http.Request) {
	var user User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user.Username == "" || user.Password == "" {
		http.Error(w, "Missing username or password", http.StatusBadRequest)
		return
	}
	var (
		username string
		password string
	)
	err = db.QueryRow("SELECT username,password FROM auth WHERE username = $1 AND password = $2", user.Username, user.Password).Scan(&username, &password)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sessionID, err := generateSessionID()
	if err != nil {
		http.Error(w, "Error generating session ID", http.StatusInternalServerError)
		return
	}
	_, err = db.Exec("INSERT INTO session (session_id, username, created_at) VALUES ($1, $2, $3)", sessionID, username, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	if r.URL.Path == "/login" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Login successful"})
	}

	// w.Header().Set("Content-Type", "application/json")
	// json.NewEncoder(w).Encode(map[string]interface{}{"message": "Login successful"})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Error(w, "Already logged out", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Error retrieving cookie", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("DELETE FROM session WHERE session_id = $1", cookie.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Expires:  time.Unix(0, 0), // Expire the cookie immediately
		MaxAge:   -1,              // Set MaxAge to -1 to delete the cookie
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	if r.URL.Path == "/logout" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Logout successful"})
	}
	// w.Header().Set("Content-Type", "application/json")
	// json.NewEncoder(w).Encode(map[string]interface{}{"message": "Logout successful"})
}
