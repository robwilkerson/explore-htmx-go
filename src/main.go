package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type ListFilter string

const (
	ALL        ListFilter = "ALL"
	INCOMPLETE ListFilter = "INCOMPLETE"
)

const (
	NoTasksMsg string = "_no-tasks"
	ViewToggle string = "_view-toggle"
	TaskList   string = "_list"
	TaskItem   string = "_todo-task"
)

type Todo struct {
	Id       string
	Complete bool
	Task     string
}

func DBConn() *sql.DB {
	db, err := sql.Open("sqlite3", "todoapp.db")
	if err != nil {
		log.Fatal("Unable to connect to the SQLite database:", err)
	}

	return db
}

func TaskCount(db *sql.DB, f ListFilter) int {
	qry := "SELECT COUNT(*) FROM todos"
	if f != ALL {
		qry = "SELECT COUNT(*) FROM todos WHERE completed = false"
	}

	// See how many we have; if we just created the first, we need to also
	// remove the empty message
	var count int
	sql, _ := db.Prepare(qry)
	sql.QueryRow().Scan(&count)

	return count
}

func main() {
	// Tell the server where to find static content
	fs := http.FileServer(http.Dir("www/assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	// Parse HTML templates
	tmpls := template.Must(template.ParseGlob("www/*.html"))

	// Helpers
	updateActionBlocks := func(w http.ResponseWriter, db *sql.DB, filter ListFilter) {
		taskCount := TaskCount(db, filter)
		tmpls.ExecuteTemplate(w, NoTasksMsg, map[string]int{"Count": taskCount})
		tmpls.ExecuteTemplate(w, ViewToggle, map[string]any{"Filter": filter, "Count": taskCount})
	}

	deleteTask := func(w http.ResponseWriter, r *http.Request) {
		// Extract the :id parameter from the path
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		db := DBConn()
		sql, _ := db.Prepare("DELETE FROM todos WHERE id = ?")
		sql.Exec(id)

		// Update the calls to action as required
		// I.e., show/hide the no tasks message, show/hide the filter toggle
		updateActionBlocks(w, db, ALL)
	}

	patchTask := func(w http.ResponseWriter, r *http.Request) {
		// PATCH /todos/:id?property=value[&property2=value2[...]]

		// Because doing more is more difficult than it's worth to me right now,
		// I'm only going to:
		//     * Handle the first property value pair that gets sent
		//     * Assume/Trust that that value is completed=(true|false)

		// Extract the :id parameter from the path
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		fmt.Printf("   ID: %s\n", id)

		qsValues := r.URL.Query()
		completed, _ := strconv.ParseBool(qsValues["completed"][0])

		db := DBConn()
		sql, err := db.Prepare("UPDATE todos SET completed = ?, updated_at = ? WHERE id = ?")
		if err != nil {
			fmt.Println("ERROR!")
			log.Fatal(err)
		}
		sql.Exec(completed, time.Now(), id)

		// No need to update any templates right now, but once we have a
		// persistent view state, we might need to decide whether we keep
		// showing an item that's marked completed
	}

	//
	// Route handlers
	//

	indexHandler := func(w http.ResponseWriter, r *http.Request) {
		var record Todo
		var data []Todo

		db := DBConn()
		records, _ := db.Query("SELECT id, task, completed FROM todos;")
		for records.Next() {
			records.Scan(&record.Id, &record.Task, &record.Complete)
			data = append(data, record)
		}

		tmpls.ExecuteTemplate(w, "index.html", map[string]any{"Tasks": data, "Filter": ALL, "Count": len(data)})
	}

	createHandler := func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		task := r.PostFormValue("task")
		complete := false // since we're just adding it

		db := DBConn()
		sql, _ := db.Prepare("INSERT INTO todos (id, task, created_at, updated_at) VALUES (?, ?, ?, ?)")
		sql.Exec(id, task, time.Now(), time.Now())
		todo := Todo{Id: id, Complete: complete, Task: task}

		// Add the new todo to the site
		fmt.Printf("Executing %s with:\n", TaskItem)
		fmt.Printf("   tasks: %v\n", todo)
		tmpls.ExecuteTemplate(w, TaskItem, todo)

		// Update the calls to action as required
		// I.e., show/hide the no tasks message, show/hide the filter toggle
		updateActionBlocks(w, db, ALL)
	}

	updatesHandler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "DELETE":
			deleteTask(w, r)
		case "PATCH":
			patchTask(w, r)
		}
	}

	filteredViewHandler := func(w http.ResponseWriter, r *http.Request) {
		// /todos/show/:filter ViewFilter

		var record Todo
		var data []Todo
		show := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:] // (ALL|INCOMPLETE)

		qry := "SELECT id, task, completed FROM todos"
		if ListFilter(show) == INCOMPLETE {
			qry = qry + " WHERE completed = false"
		}

		db := DBConn()
		records, _ := db.Query(qry)
		for records.Next() {
			records.Scan(&record.Id, &record.Task, &record.Complete)
			data = append(data, record)
		}

		// Reload the task list with the filtered results
		tmpls.ExecuteTemplate(w, TaskList, map[string]any{"Tasks": data})

		// Update the calls to action as required
		// I.e., show/hide the no tasks message, show/hide the filter toggle
		updateActionBlocks(w, db, ListFilter(show))
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/todos", createHandler)
	http.HandleFunc("/todos/", updatesHandler)
	http.HandleFunc("/todos/show/", filteredViewHandler)

	fmt.Println("Starting HTTP server on :8888...")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
