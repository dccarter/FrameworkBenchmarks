package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"sort"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)

type Message struct {
	Message string `json:"message"`
}

type World struct {
	Id           uint16 `json:"id"`
	RandomNumber uint16 `json:"randomNumber"`
}

type Fortune struct {
	Id      uint16 `json:"id"`
	Message string `json:"message"`
}

// Databases
const (
	// Go 1.4's sql.DB has scalability problem when using (explicitly reused) prepared statement.
	// https://github.com/golang/go/issues/9484
	//
	// Using db.Query() instead of stmt.Query() avoid the issue.
	// But it makes 3 round trips per query: prepare, execute and close.
	// `interpolateParams=true` enables client side parameter interpolation.
	// It reduces round trips without prepared statement.
	//
	// Before Go 1.5 is released, we can see real power of Go with this benchmark.
	// After Go 1.5 is released, we can see prepared statement vs interpolation by comparing
	// this and another lightweight Go framework.
	connectionString   = "benchmarkdbuser:benchmarkdbpass@tcp(localhost:3306)/hello_world?interpolateParams=true"
	worldSelect        = "SELECT id, randomNumber FROM World WHERE id = ?"
	worldUpdate        = "UPDATE World SET randomNumber = ? WHERE id = ?"
	fortuneSelect      = "SELECT id, message FROM Fortune;"
	worldRowCount      = 10000
	maxConnectionCount = 256
)

const helloWorldString = "Hello, World!"

var (
	// Templates
	tmpl = template.Must(template.ParseFiles("templates/layout.html", "templates/fortune.html"))

	// Database
	db *sql.DB

	helloWorldBytes = []byte(helloWorldString)
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var err error
	db, err = sql.Open("mysql", connectionString)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	db.SetMaxIdleConns(maxConnectionCount)

	http.HandleFunc("/db", dbHandler)
	http.HandleFunc("/queries", queriesHandler)
	http.HandleFunc("/json", jsonHandler)
	http.HandleFunc("/fortune", fortuneHandler)
	http.HandleFunc("/update", updateHandler)
	http.HandleFunc("/plaintext", plaintextHandler)
	http.ListenAndServe(":8080", nil)
}

// Test 1: JSON serialization
func jsonHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	json.NewEncoder(w).Encode(&Message{helloWorldString})
}

// Test 2: Single database query
func dbHandler(w http.ResponseWriter, r *http.Request) {
	var world World
	err := db.QueryRow(worldSelect, rand.Intn(worldRowCount)+1).Scan(&world.Id, &world.RandomNumber)
	if err != nil {
		log.Fatalf("Error scanning world row: %s", err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&world)
}

// Test 3: Multiple database queries
func queriesHandler(w http.ResponseWriter, r *http.Request) {
	n := 1
	if nStr := r.URL.Query().Get("queries"); len(nStr) > 0 {
		n, _ = strconv.Atoi(nStr)
	}

	if n <= 1 {
		dbHandler(w, r)
		return
	}

	world := make([]World, n)
	for i := 0; i < n; i++ {
		err := db.QueryRow(worldSelect, rand.Intn(worldRowCount)+1).Scan(&world[i].Id, &world[i].RandomNumber)
		if err != nil {
			log.Fatalf("Error scanning world row: %s", err.Error())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(world)
}

// Test 4: Fortunes
func fortuneHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(fortuneSelect)
	if err != nil {
		log.Fatalf("Error preparing statement: %v", err)
	}

	fortunes := make(Fortunes, 0, 16)
	for rows.Next() { //Fetch rows
		fortune := Fortune{}
		if err := rows.Scan(&fortune.Id, &fortune.Message); err != nil {
			log.Fatalf("Error scanning fortune row: %s", err.Error())
		}
		fortunes = append(fortunes, &fortune)
	}
	rows.Close()
	fortunes = append(fortunes, &Fortune{Message: "Additional fortune added at request time."})

	sort.Sort(ByMessage{fortunes})
	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, fortunes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Test 5: Database updates
func updateHandler(w http.ResponseWriter, r *http.Request) {
	n := 1
	if nStr := r.URL.Query().Get("queries"); len(nStr) > 0 {
		n, _ = strconv.Atoi(nStr)
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)

	if n <= 1 {
		var world World
		db.QueryRow(worldSelect, rand.Intn(worldRowCount)+1).Scan(&world.Id, &world.RandomNumber)
		world.RandomNumber = uint16(rand.Intn(worldRowCount) + 1)
		db.Exec(worldUpdate, world.RandomNumber, world.Id)
		encoder.Encode(&world)
	} else {
		world := make([]World, n)
		for i := 0; i < n; i++ {
			if err := db.QueryRow(worldSelect, rand.Intn(worldRowCount)+1).Scan(&world[i].Id, &world[i].RandomNumber); err != nil {
				log.Fatalf("Error scanning world row: %s", err.Error())
			}
			world[i].RandomNumber = uint16(rand.Intn(worldRowCount) + 1)
			if _, err := db.Exec(worldUpdate, world[i].RandomNumber, world[i].Id); err != nil {
				log.Fatalf("Error updating world row: %s", err.Error())
			}
		}
		encoder.Encode(world)
	}
}

// Test 6: Plaintext
func plaintextHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write(helloWorldBytes)
}

type Fortunes []*Fortune

func (s Fortunes) Len() int      { return len(s) }
func (s Fortunes) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByMessage struct{ Fortunes }

func (s ByMessage) Less(i, j int) bool { return s.Fortunes[i].Message < s.Fortunes[j].Message }
