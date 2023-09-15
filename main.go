package main

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type App struct {
	Port       string
	StaticBase string
}
type Todo struct {
	Id       string
	Complete bool
	Task     string
}
type TaskCount struct {
	Total      int
	Incomplete int
}

type View string

const (
	ALL        View = "ALL"
	INCOMPLETE View = "INCOMPLETE"
)

type TemplatePartial string

const (
	NoTasksMsg TemplatePartial = "_no-tasks"
	ViewToggle TemplatePartial = "_view-toggle"
	TaskList   TemplatePartial = "_list"
	TaskItem   TemplatePartial = "_todo-task"
)

// Start launches the HTTP server and defines its route handlers
func (a *App) Start() {
	if a.StaticBase == "/assets" {
		log.Printf("Serving static assets")
		http.Handle("/assets/", logAccess(staticHandler("www/assets")))
	}

	// Route handlers
	http.Handle("/", logAccess(a.indexHandler))
	http.Handle("/todos", logAccess(writeHandler))
	http.Handle("/todos/", logAccess(writeHandler))
	http.Handle("/todos/show/", logAccess(updateView))

	// Start the HTTP service
	port := fmt.Sprintf(":%s", a.Port)
	log.Printf("Starting app on %s", port)
	log.Printf("Access the app at http://localhost:%s", a.Port)
	log.Fatal(http.ListenAndServe(port, nil))
}

// indexHandler displays the homepage. It is attached to the App so that we can
// pass the STATIC_BASE to the template in order to render static assets.
func (a App) indexHandler(w http.ResponseWriter, r *http.Request) {
	var res struct {
		App   App
		Tasks []Todo
		Show  View // ALL|INCOMPLETE
		Count TaskCount
	}
	var record Todo

	if r.URL.Path != "/" {
		fmt.Printf("%s requested, 404 returned\n", r.URL.Path)
		http.Error(w, "Not Found", 404)
		return
	}

	// Check/Set a cookie with the current view of the list; default to ALL
	cookie, err := r.Cookie("View")
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			fmt.Printf("%s No View cookie found in the request\n", r.URL.Path)
			// set cookie
			cookie = &http.Cookie{
				Name:   "View",
				Value:  string(ALL),
				Path:   "/",
				MaxAge: 86400,
			}
			fmt.Printf("%s Setting View cookie: %s\n", r.URL.Path, cookie.Value)
			http.SetCookie(w, cookie)
		} else {
			// Fatal
			fmt.Println(err)
			http.Error(w, "Server error reading cookie", http.StatusInternalServerError)
		}
	}

	qry := "SELECT id, task, completed FROM todos"
	if View(cookie.Value) == INCOMPLETE {
		qry = qry + " WHERE completed = false"
	}

	db := DBConn()
	records, _ := db.Query(qry)
	for records.Next() {
		records.Scan(&record.Id, &record.Task, &record.Complete)
		res.Tasks = append(res.Tasks, record)
	}

	res.App = a
	res.Show = View(cookie.Value)
	res.Count = getCounts(db)

	renderTemplate(w, "index.html", res)
}

// writeHandler answers and delegates requests sent with write methods (POST,
// PUT, PATH, DELETE)
func writeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		createTask(w, r)
	case "PATCH":
		patchTask(w, r)
	case "DELETE":
		deleteTask(w, r)
	}
}

// createTask adds a new task to the todo list
func createTask(w http.ResponseWriter, r *http.Request) {
	id := uuid.NewString()
	task := r.PostFormValue("task")
	complete := false // Makes no sense to add it if it's already complete

	db := DBConn()
	sql, _ := db.Prepare("INSERT INTO todos (id, task, created_at, updated_at) VALUES (?, ?, ?, ?)")
	sql.Exec(id, task, time.Now(), time.Now())
	todo := Todo{Id: id, Complete: complete, Task: task}

	// Add the new todo to the list
	renderTemplate(w, string(TaskItem), todo)

	// Update the calls to action as required
	// I.e., show/hide the no tasks message, show/hide the filter toggle
	updateSwapsFromState(w, r, db)
}

// patchTask updates a single property of a task. For our purposes, that means
// updating its completed status
func patchTask(w http.ResponseWriter, r *http.Request) {
	// PATCH /todos/:id?property=value[&property2=value2[...]]

	// Because doing more is more difficult than it's worth to me right now,
	// I'm only going to:
	//     * Handle the first property value pair that gets sent
	//     * Assume/Trust that that value is completed=(true|false)

	// Extract the :id parameter from the path
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

	qsValues := r.URL.Query()
	completed, _ := strconv.ParseBool(qsValues["completed"][0])

	db := DBConn()
	sql, _ := db.Prepare("UPDATE todos SET completed = ?, updated_at = ? WHERE id = ?")
	sql.Exec(completed, time.Now(), id)

	// If displaying INCOMPLETE only, but marking this complete, we need to
	// adjust re-render the display
	c, _ := r.Cookie("View")
	if View(c.Value) == INCOMPLETE && completed {
		var data struct {
			Tasks []Todo
		}
		data.Tasks = getTasks(db, View(c.Value))
		renderTemplate(w, string(TaskList), data)
	}

	updateSwapsFromState(w, r, db)
}

// deleteTask destroys all existence of a task
func deleteTask(w http.ResponseWriter, r *http.Request) {
	// Extract the :id parameter from the path
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

	db := DBConn()
	sql, _ := db.Prepare("DELETE FROM todos WHERE id = ?")
	sql.Exec(id)

	// Update the calls to action as required
	// I.e., show/hide the no tasks message, show/hide the filter toggle
	updateSwapsFromState(w, r, db)
}

// updateView resets the list view to show either all tasks or only those that
// are incomplete
func updateView(w http.ResponseWriter, r *http.Request) {
	// /todos/show/:filter View
	var data struct {
		Tasks []Todo
	}

	db := DBConn()
	show := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:] // (ALL|INCOMPLETE)
	data.Tasks = getTasks(db, View(show))

	// Update the cookie with the new View value
	cookie := &http.Cookie{
		Name:   "View",
		Value:  show,
		Path:   "/",
		MaxAge: 86400,
	}
	http.SetCookie(w, cookie)

	// Reload the task list with the filtered results
	renderTemplate(w, string(TaskList), data)

	// Update the calls to action as required
	// I.e., show/hide the no tasks message, show/hide the filter toggle
	updateSwaps(w, db, View(cookie.Value))
}

//
// Helper Functions
//

// getTasks returns a complete list of tasks to be displayed based on the
// current view selection (ALL|INCOMPLETE)
func getTasks(db *sql.DB, f View) []Todo {
	var record Todo
	var tasks []Todo

	qry := "SELECT id, task, completed FROM todos"
	if f == INCOMPLETE {
		qry = qry + " WHERE completed = false"
	}

	// Fetch the records matching the View
	records, _ := db.Query(qry)
	for records.Next() {
		records.Scan(&record.Id, &record.Task, &record.Complete)
		tasks = append(tasks, record)
	}

	return tasks
}

// getCounts returns information about the how many items have been created and
// how many are incomplete
func getCounts(db *sql.DB) TaskCount {
	qry := "SELECT COUNT(*) FROM todos"

	var totalCount int
	sql, _ := db.Prepare(qry)
	sql.QueryRow().Scan(&totalCount)

	incompleteCount := totalCount
	qry = "SELECT COUNT(*) FROM todos WHERE completed = false"
	sql, _ = db.Prepare(qry)
	sql.QueryRow().Scan(&incompleteCount)

	return TaskCount{Total: totalCount, Incomplete: incompleteCount}
}

// updateActionBlocksFromState is called when we can trust the cookie value
// because the primary action made no change to that cookie value
func updateSwapsFromState(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	filter, _ := r.Cookie("View")

	updateSwaps(w, db, View(filter.Value))
}

// updateSwaps renders the template partials (really template blocks) whose
// "out-of-band" content needs to be modified based on changes made in the
// process
func updateSwaps(w http.ResponseWriter, db *sql.DB, v View) {
	// data defines the data structure that will be sent to the template partial
	var data struct {
		Show  View
		Count TaskCount
	}
	data.Show = v
	data.Count = getCounts(db)

	renderTemplate(w, string(NoTasksMsg), data)
	renderTemplate(w, string(ViewToggle), data)
}

// renderTemplate renders a specific template with data
func renderTemplate(w http.ResponseWriter, name string, data any) {
	// This is inefficient - it reads the templates from the
	// filesystem every time. This makes it much easier to
	// develop though, so I can edit my templates and the
	// changes will be reflected without having to restart
	// the app.
	t, err := template.ParseGlob("www/*.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error %s", err.Error()), 500)
		return
	}

	err = t.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error %s", err.Error()), 500)
		return
	}
}

//
// Service Functions
//

// DBConn returns a database connection ready to use
func DBConn() *sql.DB {
	db, err := sql.Open("sqlite3", "todoapp.db")
	if err != nil {
		log.Fatal("Unable to connect to the SQLite database:", err)
	}

	return db
}

// staticHandler returns a function that handles access to static assets
func staticHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/assets/", http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
	}
}

// logAccess wraps a HandlerFunc to create an access log record for every
// incoming request
func logAccess(f func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)

		f(w, r)
	})
}

func env(key, defaultValue string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}

	return v
}

func main() {
	server := App{
		Port:       env("PORT", "8080"),
		StaticBase: env("STATIC_BASE", "/static"),
	}
	server.Start()
}
