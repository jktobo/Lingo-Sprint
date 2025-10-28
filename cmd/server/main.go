package main

import (
	"log"
	"net/http"

	"lingo-sprint/internal/api"
	"lingo-sprint/internal/database"

	"github.com/gorilla/mux"
)

func main() {
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("Could not connect to the database: %v", err)
	}
	log.Println("Successfully connected to the database!")
	defer db.Close()

	r := mux.NewRouter()
	apiHandler := api.NewApiHandler(db)

	// === ИЗМЕНЕНИЯ ЗДЕСЬ ===

	// 1. Создаем /api саб-роутер
	apiRouter := r.PathPrefix("/api").Subrouter()

	// 2. Эти ручки НЕ защищены. Они нужны для входа.
	apiRouter.HandleFunc("/register", apiHandler.RegisterUser).Methods("POST")
	apiRouter.HandleFunc("/login", apiHandler.LoginUser).Methods("POST")

	// 3. Создаем "защищенный" саб-роутер
	// Все, что будет зарегистрировано в 's', будет проходить через AuthMiddleware
	s := apiRouter.PathPrefix("/").Subrouter()
	s.Use(api.AuthMiddleware) // <-- "Охранник" подключен!

	// 4. Регистрируем все остальные ручки в 's'
	s.HandleFunc("/levels", apiHandler.GetLevels).Methods("GET")
	s.HandleFunc("/levels/{level_id:[0-9]+}/lessons", apiHandler.GetLessonsByLevel).Methods("GET")
	s.HandleFunc("/lessons/{lesson_id:[0-9]+}/sentences", apiHandler.GetSentencesByLesson).Methods("GET")
	s.HandleFunc("/progress/save", apiHandler.SaveProgress).Methods("POST")

	s.HandleFunc("/ai/explain-error", apiHandler.ExplainError).Methods("POST")

	// ========================




	// Ручка для обслуживания нашего фронтенда
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/static/")))
	
	port := ":8080"
	log.Printf("Starting server on port %s", port)
	
	loggedRouter := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s %s", req.RemoteAddr, req.Method, req.URL)
		r.ServeHTTP(w, req) 
	})
	
	if err := http.ListenAndServe(port, loggedRouter); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}