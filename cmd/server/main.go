package main

import (
	"log"
	"net/http"
	"strings" // Нужен для HasPrefix

	"lingo-sprint/internal/api"
	"lingo-sprint/internal/database"

	// "github.com/golang-jwt/jwt/v5" // <-- ЭТОТ ИМПОРТ НЕ НУЖЕН ЗДЕСЬ
	"github.com/gorilla/mux"
)

// isLoggedIn проверяет простой cookie 'auth_status'.
func isLoggedIn(r *http.Request) bool {
	log.Println("--- Checking isLoggedIn ---")
	cookie, err := r.Cookie("auth_status")
	if err != nil {
		log.Println("isLoggedIn: Cookie 'auth_status' not found. Returning false.")
		return false
	}
	isLoggedInResult := cookie.Value == "logged_in"
	log.Printf("isLoggedIn: Cookie 'auth_status' found. Value='%s'. Returning %t.", cookie.Value, isLoggedInResult)
	return isLoggedInResult
}


func main() {
	db, err := database.Connect()
	if err != nil { log.Fatalf("DB connect error: %v", err) }
	log.Println("DB connected!")
	defer db.Close()

	r := mux.NewRouter()
	apiHandler := api.NewApiHandler(db)

	// --- 1. РЕГИСТРАЦИЯ API ЭНДПОИНТОВ ---
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.HandleFunc("/register", apiHandler.RegisterUser).Methods("POST")
	apiRouter.HandleFunc("/login", apiHandler.LoginUser).Methods("POST")

	s := apiRouter.PathPrefix("/").Subrouter()
	s.Use(api.AuthMiddleware)
	s.HandleFunc("/levels", apiHandler.GetLevels).Methods("GET")
	s.HandleFunc("/levels/{level_id:[0-9]+}/lessons", apiHandler.GetLessonsByLevel).Methods("GET")
	s.HandleFunc("/lessons/{lesson_id:[0-9]+}/sentences", apiHandler.GetSentencesByLesson).Methods("GET")
	s.HandleFunc("/progress/save", apiHandler.SaveProgress).Methods("POST")
	// --- УБЕДИТЕСЬ, ЧТО ЭТО РАСКОММЕНТИРОВАНО ---
	s.HandleFunc("/ai/explain-error", apiHandler.ExplainError).Methods("POST")

	// --- 2. РЕГИСТРАЦИЯ СТРАНИЦ ПРИЛОЖЕНИЯ ---
	r.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		if !isLoggedIn(r) {
			log.Println("Redirecting /app to / (not logged in)")
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		log.Println("Serving /app (index.html)")
		http.ServeFile(w, r, "./web/static/index.html")
	}).Methods("GET")
    // r.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request){
    //     log.Println("Redirecting /login to /")
    //     http.Redirect(w, r, "/", http.StatusSeeOther)
    // }).Methods("GET")
    // r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request){
    //      log.Println("Redirecting /register to /")
    //      http.Redirect(w, r, "/", http.StatusSeeOther)
    // }).Methods("GET")

	// --- 3. РЕГИСТРАЦИЯ ЛЕНДИНГА ---
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isLoggedIn(r) {
			log.Println("Redirecting / to /app (already logged in)")
			http.Redirect(w, r, "/app", http.StatusSeeOther)
			return
		}
		log.Println("Serving / (landing.html)")
		http.ServeFile(w, r, "./web/static/landing.html")
	}).Methods("GET")

	// // --- 4. РЕГИСТРАЦИЯ FILESERVER ДЛЯ СТАТИКИ ---
	// fs := http.FileServer(http.Dir("./web/static/"))
	// r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
	// 	path := r.URL.Path
	// 	isStatic := path != "/" && path != "/app" && path != "/login" && path != "/register" && !strings.HasPrefix(path, "/api")
	// 	return isStatic
	// }).Handler(http.StripPrefix("/", fs))

	// --- 4. РЕГИСТРАЦИЯ FILESERVER ДЛЯ /media (НОВОЕ) ---
	// Этот код будет обслуживать ваши MP3-файлы
	mediaFs := http.FileServer(http.Dir("./media/"))
	r.PathPrefix("/media/").Handler(http.StripPrefix("/media/", mediaFs))

	// --- 5. РЕГИСТРАЦИЯ FILESERVER ДЛЯ СТАТИКИ (СТАРОЕ, теперь 5) ---
	// Этот код обслуживает ваш index.html, app.js, style.css и т.д.
	fs := http.FileServer(http.Dir("./web/static/"))
	r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		path := r.URL.Path
		isStatic := path != "/" &&
			path != "/app" &&
			path != "/login" &&
			path != "/register" &&
			!strings.HasPrefix(path, "/api") &&
			!strings.HasPrefix(path, "/media") // <-- Добавлено
		return isStatic
	}).Handler(http.StripPrefix("/", fs))


	// --- ЗАПУСК СЕРВЕРА ---
	port := ":8080"
	log.Printf("Starting server on port %s", port)
	loggedRouter := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s %s", req.RemoteAddr, req.Method, req.URL)
		r.ServeHTTP(w, req)
	})
	if err := http.ListenAndServe(port, loggedRouter); err != nil {
		log.Fatalf("Server start error: %v", err)
	}
}
