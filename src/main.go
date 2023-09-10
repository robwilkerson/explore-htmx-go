package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Todo struct {
	Id       string
	Complete bool
	Task     string
}
type ResponseData struct {
	Tasks []Todo
	View  string
}

func DBConn() *sql.DB {
	db, err := sql.Open("sqlite3", "todoapp.db")
	if err != nil {
		log.Fatal("Unable to connect to the SQLite database:", err)
	}

	return db
}

func main() {
	// Tell the server where to find static content
	fs := http.FileServer(http.Dir("www/assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	// Parse HTML templates
	tmpls := template.Must(template.ParseGlob("www/*.html"))

	// Helpers
	filteredList := func(w http.ResponseWriter, showCompleted bool) {
		var data ResponseData
		var record Todo

		qry := "SELECT id, task, completed FROM todos"
		data.View = "ALL"
		if !showCompleted {
			qry = "SELECT id, task, completed FROM todos WHERE completed = false;"
			data.View = "INCOMPLETE"
		}

		db := DBConn()
		records, _ := db.Query(qry)
		for records.Next() {
			records.Scan(&record.Id, &record.Task, &record.Complete)
			data.Tasks = append(data.Tasks, record)
		}

		tmpls.ExecuteTemplate(w, "_toggle_completed", data.View)
		tmpls.ExecuteTemplate(w, "_list", data.Tasks)
	}

	//
	// Route handlers
	//

	listHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("HX-Request") == "true" {
			show, _ := strconv.ParseBool(r.URL.Query().Get("showcompleted"))
			filteredList(w, show)
			return
		}

		var data ResponseData
		var record Todo

		data.View = "ALL"

		db := DBConn()
		records, _ := db.Query("SELECT id, task, completed FROM todos;")
		for records.Next() {
			records.Scan(&record.Id, &record.Task, &record.Complete)
			data.Tasks = append(data.Tasks, record)
		}

		tmpls.ExecuteTemplate(w, "index.html", data)
	}

	createHandler := func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		task := r.PostFormValue("task")
		complete := false // since we're just adding it

		var data ResponseData

		db := DBConn()
		sql, _ := db.Prepare("INSERT INTO todos (id, task, created_at, updated_at) VALUES (?, ?, ?, ?)")
		sql.Exec(id, task, time.Now(), time.Now())
		todo := Todo{Id: id, Complete: complete, Task: task}

		tmpls.ExecuteTemplate(w, "todo-task", todo)
		fmt.Println("task template executed!")


		// See how many we have; if we just created the first, we need to also
		// remove the empty message
		var count int
		sql, _ = db.Prepare("SELECT COUNT(*) FROM todos")
		sql.QueryRow().Scan(&count)

		fmt.Printf("TODO count: %d\n", count)

		// The first added removes the empty list statement in favor of a
		// show/hide call to action
		if count == 1 {
			fmt.Println("Created our first task!")
			data.Tasks = append(data.Tasks, todo)
			data.View = "ALL"
			tmpls.ExecuteTemplate(w, "_toggle_completed", data)
			fmt.Println("toggle template executed!")
		}
	}

	completedHandler := func(w http.ResponseWriter, r *http.Request) {
		id := r.PostFormValue("id")
		completed := true
		if r.PostFormValue("completed") == "" {
			completed = false
		}

		db := DBConn()
		sql, _ := db.Prepare("UPDATE todos SET completed = ?, updated_at = ? WHERE id = ?")
		sql.Exec(completed, time.Now(), id)
	}

	http.HandleFunc("/", listHandler)
	http.HandleFunc("/todos", createHandler)
	http.HandleFunc("/todos/complete", completedHandler)

	fmt.Println("Starting HTTP server on :8888...")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
