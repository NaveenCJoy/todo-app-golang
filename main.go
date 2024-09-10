package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/thedevsaddam/renderer"
)

var rnd *renderer.Render
var db *mgo.Database

const (
	hostName       string = "localhost:27017"
	dbName         string = "demo_todo"
	collectionName string = "todo"
	port           string = ":9000"
)

type todoModel struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	Title     string        `bson:"title"`
	Completed bool          `bson:"completed"`
	CreatedAt time.Time     `bson:"createAt"`
}

type todo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"createAt"`
}

// for connecting with db
func init() {
	rnd = renderer.New()
	sess, err := mgo.Dial(hostName)
	checkErr(err)
	sess.SetMode(mgo.Monotonic, true)
	db = sess.DB(dbName)
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	err := rnd.Template(w, http.StatusOK, []string{"/static/home.tpl"}, nil)
	checkErr(err)
}

func fetchTodos(w http.ResponseWriter, r *http.Request) {
	todos := []todoModel{}
	//todoModel - bson data
	//todo - json data

	err := db.C(collectionName).Find(bson.M{}).All(&todos)
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to fetch todos",
			"error":   err,
		})
		return
	}
	todoList := []todo{}

	for _, t := range todos {
		todoList = append(todoList, todo{
			ID:        t.ID.Hex(),
			Title:     t.Title,
			Completed: t.Completed,
			CreatedAt: t.CreatedAt,
		})
	}
	rnd.JSON(w, http.StatusOK, renderer.M{
		"data": todoList,
	})
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	var t todo

	//pass r.Body to Decoder, store decoded data to var t
	//step1 - take input
	err := json.NewDecoder(r.Body).Decode(&t)
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}
	//step2 - check if the input have title
	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Fuck You!",
		})
		return
	}
	//step3 - convert todo to todoModel (to send to db)
	tm := todoModel{
		ID:        bson.NewObjectId(),
		Title:     t.Title,
		Completed: false,
		CreatedAt: time.Now(),
	}
	//step4 - send todo to db
	dbErr := db.C(collectionName).Insert(&tm)
	if dbErr != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to save todo",
			"error":   dbErr,
		})
		return
	}
	//send a response to frontend
	rnd.JSON(w, http.StatusCreated, renderer.M{
		"message": "Todo Created Successfully",
		"Todo ID": tm.ID.Hex(),
	})

}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	//get id from params
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	// check if id is hex
	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Invalid id",
		})
		return
	}

	var t todo

	err := json.NewDecoder(r.Body).Decode(&t)
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}

	if t.Title == "" {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Title is requried",
		})
		return
	}

	dbErr := db.C(collectionName).Update(
		bson.M{"_id": bson.ObjectIdHex(id)},
		bson.M{"title": t.Title, "Completed": t.Completed},
	)
	if dbErr != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to update todo",
			"error":   dbErr,
		})
	}

}

func deleteTodo(w http.ResponseWriter, r *http.Request) {
	//get id from params
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	//check if id is hex
	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Invalid ID",
		})
		return
	}
	//remove todo from db
	err := db.C(collectionName).RemoveId(bson.ObjectIdHex(id))
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to remove todo",
			"error":   err,
		})
		return
	}
	//send a response to frontend
	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todo deleted successfully",
	})
}

func main() {
	//when an os.Interrupt signal is detected, it will be send to stopchan
	stopChan := make(chan os.Signal)
	signal.Notify(stopChan, os.Interrupt)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", homeHandler)
	r.Mount("/todo", todoHandlers())

	// srv is server
	srv := &http.Server{
		Addr:         port,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("Listening on port", port)
		err := srv.ListenAndServe()
		if err != nil {
			log.Printf("Listen: %s\n", err)
		}
	}()

	<-stopChan
	fmt.Println("Shutting Down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	srv.Shutdown(ctx)

	defer cancel()
	log.Println("Server successfully closed")

	// go func() {
	// 	log.Println("Listening on port", port)
	// 	if err := srv.ListenAndServe(); err != nil {
	// 		log.Printf("Listen: %s\n", err)
	// 	}
	// }()
}

func todoHandlers() http.Handler {
	rg := chi.NewRouter()
	rg.Group(func(r chi.Router) {
		r.Get("/", fetchTodos)
		r.Post("/", createTodo)
		r.Put("/{id}", updateTodo)
		r.Delete("/{id}", deleteTodo)
	})
	return rg
}
