package main

import (
	"database/sql"
	"errors"
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

func TaskCount(db *sql.DB) map[string]int {
	qry := "SELECT COUNT(*) FROM todos"

	var totalCount int
	sql, _ := db.Prepare(qry)
	sql.QueryRow().Scan(&totalCount)

	incompleteCount := totalCount
	qry = "SELECT COUNT(*) FROM todos WHERE completed = false"
	sql, _ = db.Prepare(qry)
	sql.QueryRow().Scan(&incompleteCount)

	return map[string]int{"total": totalCount, "incomplete": incompleteCount}
}

func main() {
	// Tell the server where to find static content
	fs := http.FileServer(http.Dir("www/assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	// Parse HTML templates
	tmpls := template.Must(template.ParseGlob("www/*.html"))

	//
	// Helpers
	//

	getTasks := func(db *sql.DB, f ListFilter) []Todo {
		var record Todo
		var tasks []Todo

		qry := "SELECT id, task, completed FROM todos"
		if f == INCOMPLETE {
			qry = qry + " WHERE completed = false"
		}

		// Fetch the records matching the ListFilter
		records, _ := db.Query(qry)
		for records.Next() {
			records.Scan(&record.Id, &record.Task, &record.Complete)
			tasks = append(tasks, record)
		}

		return tasks
	}

	updateActionBlocks := func(w http.ResponseWriter, db *sql.DB, f ListFilter) {
		taskCount := TaskCount(db)
		tmpls.ExecuteTemplate(w, NoTasksMsg, map[string]map[string]int{"Count": taskCount})
		tmpls.ExecuteTemplate(w, ViewToggle, map[string]any{"Filter": f, "Count": taskCount})
	}

	// updateActionBlocksFromState is called when we can trust the cookie value
	updateActionBlocksFromState := func(w http.ResponseWriter, r *http.Request, db *sql.DB) {
		// Find out what we're displaying
		filter, _ := r.Cookie("ListFilter")

		updateActionBlocks(w, db, ListFilter(filter.Value))
	}

	deleteTask := func(w http.ResponseWriter, r *http.Request) {
		// Extract the :id parameter from the path
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		db := DBConn()
		sql, _ := db.Prepare("DELETE FROM todos WHERE id = ?")
		sql.Exec(id)

		// Update the calls to action as required
		// I.e., show/hide the no tasks message, show/hide the filter toggle
		updateActionBlocksFromState(w, r, db)
	}

	patchTask := func(w http.ResponseWriter, r *http.Request) {
		// PATCH /todos/:id?property=value[&property2=value2[...]]

		// Because doing more is more difficult than it's worth to me right now,
		// I'm only going to:
		//     * Handle the first property value pair that gets sent
		//     * Assume/Trust that that value is completed=(true|false)

		// Extract the :id parameter from the path
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		qsValues := r.URL.Query()
		completed, _ := strconv.ParseBool(qsValues["completed"][0])

		fmt.Printf("%s Updating task %s to completed status of %t\n", r.URL.Path, id, completed)

		db := DBConn()
		sql, err := db.Prepare("UPDATE todos SET completed = ?, updated_at = ? WHERE id = ?")
		if err != nil {
			fmt.Println("ERROR!")
			log.Fatal(err)
		}
		sql.Exec(completed, time.Now(), id)

		// If displaying INCOMPLETE only, but marking this complete, we need to
		// adjust re-render the display
		c, _ := r.Cookie("ListFilter")
		if ListFilter(c.Value) == INCOMPLETE && completed {
			fmt.Printf("%s Updating the task list", r.URL.Path)
			tmpls.ExecuteTemplate(w, TaskList, map[string]any{"Tasks": getTasks(db, ListFilter(c.Value))})
		}

		updateActionBlocksFromState(w, r, db)
	}

	//
	// Route handlers
	//

	indexHandler := func(w http.ResponseWriter, r *http.Request) {
		var record Todo
		var data []Todo

		if r.URL.Path != "/" {
			fmt.Printf("%s requested, 404 returned\n", r.URL.Path)
			http.Error(w, "Not Found", 404)
			return
		}

		// Check/Set a cookie with the current view of the list; default to ALL
		cookie, err := r.Cookie("ListFilter")
		if err != nil {
			if errors.Is(err, http.ErrNoCookie) {
				fmt.Printf("%s No ListFilter cookie found in the request\n", r.URL.Path)
				// set cookie
				cookie = &http.Cookie{
					Name:   "ListFilter",
					Value:  string(ALL),
					Path:   "/",
					MaxAge: 86400,
				}
				fmt.Printf("%s Setting ListFilter cookie: %s\n", r.URL.Path, cookie.Value)
				http.SetCookie(w, cookie)
			} else {
				// Fatal
				fmt.Println(err)
				http.Error(w, "Server error reading cookie", http.StatusInternalServerError)
			}
		}

		qry := "SELECT id, task, completed FROM todos"
		if ListFilter(cookie.Value) == INCOMPLETE {
			qry = qry + " WHERE completed = false"
		}

		db := DBConn()
		records, _ := db.Query(qry)
		for records.Next() {
			records.Scan(&record.Id, &record.Task, &record.Complete)
			data = append(data, record)
		}

		tmpls.ExecuteTemplate(w, "index.html", map[string]any{"Tasks": data, "Filter": cookie.Value, "Count": TaskCount(db)})
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
		updateActionBlocksFromState(w, r, db)
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

		db := DBConn()
		show := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:] // (ALL|INCOMPLETE)
		data := getTasks(db, ListFilter(show))

		// Update the cookie with the new ListFilter value
		cookie := &http.Cookie{
			Name:   "ListFilter",
			Value:  show,
			Path:   "/",
			MaxAge: 86400,
		}
		fmt.Printf("%s Setting ListFilter cookie: %s\n", r.URL.Path, cookie.Value)
		http.SetCookie(w, cookie)

		// Reload the task list with the filtered results
		tmpls.ExecuteTemplate(w, TaskList, map[string]any{"Tasks": data})

		// Update the calls to action as required
		// I.e., show/hide the no tasks message, show/hide the filter toggle
		updateActionBlocks(w, db, ListFilter(cookie.Value))
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/todos", createHandler)
	http.HandleFunc("/todos/", updatesHandler)
	http.HandleFunc("/todos/show/", filteredViewHandler)

	fmt.Println("Starting HTTP server on :8888...")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
