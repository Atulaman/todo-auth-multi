package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"todo-auth/authetntication"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"
)

type task struct {
	Id   int    `json:"id"`
	Desc string `json:"desc"`
}

var db *sql.DB

func caller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		cookie, err := r.Cookie("session_id")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Error(w, "Unauthorized user", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Error retrieving cookie", http.StatusInternalServerError)
			return
		}
		var username string
		var created_at time.Time
		err = db.QueryRow("SELECT username, created_at FROM session WHERE session_id = $1", cookie.Value).Scan(&username, &created_at)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Unauthorized user", http.StatusUnauthorized)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		duration := time.Now().UTC().Sub(created_at) //time.Since(created_at)
		if duration >= 5*time.Minute {
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
			http.Error(w, "Unauthorized user", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func connect() {
	connStr := "host=localhost port=5432 user=postgres password=rx dbname=todo-auth sslmode=disable"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
	}
	fmt.Println("Connected to the database successfully!")
	authetntication.SetDB(db)
}
func apiroutes() http.Handler {
	r := chi.NewRouter()
	r.Use(caller)
	r.Route("/tasks", func(r chi.Router) {
		r.Post("/", add)
		r.Get("/", list)
		r.Put("/", update)
		r.Delete("/", delete)
	})
	return r

}
func main() {
	connect()
	defer db.Close()
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Post("/login", authetntication.Login)
	r.Post("/register", authetntication.Register)
	r.Post("/logout", authetntication.Logout)
	r.Mount("/", apiroutes())

	fmt.Println("Server is running on http://localhost:8081")
	log.Fatal(http.ListenAndServe(":8081", r))
}
func add(w http.ResponseWriter, r *http.Request) {
	var newTask task
	err := json.NewDecoder(r.Body).Decode(&newTask)
	//fmt.Print(newTask)

	if err != nil || newTask.Desc == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	cookie, _ := r.Cookie("session_id")
	var username string
	err = db.QueryRow(`SELECT username FROM session WHERE session_id=$1`, cookie.Value).Scan(&username)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Unauthorized user", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p, err := db.Query(`SELECT
	CASE
	WHEN (SELECT id FROM "Tasks" WHERE id=1 AND username=$1) IS NULL THEN 1
	ELSE
	(select coalesce(min(t1.id +1),1) from (SELECT id FROM "Tasks" WHERE username=$1) t1 left join (SELECT id FROM "Tasks" WHERE username=$1) t2 on t1.id +1 =t2.id  where t2.id is null)
	END
	`, username)
	// 	p, err := db.Query(`SELECT
	// CASE
	// WHEN (SELECT t1.id FROM "Tasks" t1 JOIN session s ON t1.username=s.username WHERE id=1 AND s.session_id=$1 ) IS NULL THEN 1
	// ELSE
	// (select coalesce(min(t1.id +1),1) from (SELECT t1.id FROM "Tasks" t1 JOIN session s ON t1.username=s.username WHERE s.session_id=$1) t1 left join (SELECT t1.id FROM "Tasks" t1 JOIN session s ON t1.username=s.username WHERE s.session_id=$1) t2 on t1.id +1 =t2.id where t2.id is null)
	// END`, cookie.Value)
	if err != nil {
		http.Error(w, "Error while generating id", http.StatusInternalServerError)
		//log.Fatal(err)
		return
	}
	p.Next()
	if err := p.Scan(&newTask.Id); err != nil {
		log.Fatal(err)
	}

	result, err := db.Exec(`INSERT INTO "Tasks" (id,description,username) VALUES ($1,$2,$3)`, newTask.Id, newTask.Desc, username)
	if err != nil {
		http.Error(w, "Error while adding task", http.StatusInternalServerError)
		return
		//log.Fatal(err)
	}
	fmt.Println(result)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Task added successfully", "task": newTask})
}
func list(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie("session_id")
	//rows, err := db.Query(`SELECT id, description FROM "Tasks" ORDER BY id ASC`)
	rows, err := db.Query(`SELECT t1.id,t1.description FROM "Tasks" t1 JOIN session a ON t1.username=a.username where a.session_id=$1 ORDER BY t1.id ASC`, cookie.Value)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	var tasks []task
	for rows.Next() {
		var t task
		if err := rows.Scan(&t.Id, &t.Desc); err != nil {
			log.Fatal(err)
			return
		}
		tasks = append(tasks, t)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if len(tasks) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "No tasks found", "count": 0, "tasks": []task{}})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"tasks": tasks, "message": "success", "count": len(tasks)})
}
func update(w http.ResponseWriter, r *http.Request) {
	var newTask task
	err := json.NewDecoder(r.Body).Decode(&newTask)
	if err != nil || newTask.Id <= 0 || newTask.Desc == "" {
		http.Error(w, "Invalid Id or Description", http.StatusBadRequest)
		return
	}
	cookie, _ := r.Cookie("session_id")
	var username string
	err = db.QueryRow(`SELECT username FROM session WHERE session_id=$1`, cookie.Value).Scan(&username)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Unauthorized user", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := db.Exec(`UPDATE "Tasks" SET description=$2 WHERE id=$1 AND username=$3`, newTask.Id, newTask.Desc, username)
	if err != nil {
		http.Error(w, "Error while updating task", http.StatusInternalServerError)
		return
	}
	//fmt.Println(result)
	RowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Fatal(err)
		//http.Error(w, "Database error", http.StatusInternalServerError)
	}
	if RowsAffected == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Task Updated successfully", "task": newTask})
}
func delete(w http.ResponseWriter, r *http.Request) {
	var newTask task
	err := json.NewDecoder(r.Body).Decode(&newTask)
	if err != nil || newTask.Id <= 0 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	cookie, _ := r.Cookie("session_id")
	var username string
	err = db.QueryRow(`SELECT username FROM session WHERE session_id=$1`, cookie.Value).Scan(&username)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Unauthorized user", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := db.Exec(`DELETE FROM "Tasks" WHERE id=$1 AND username=$2`, newTask.Id, username)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Fatal(err)
	}
	RowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Fatal(err)
	}
	if RowsAffected == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "Task not found!"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Task has been deleted successfully!"})
}
