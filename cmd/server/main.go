package main

import (
	"log"
	"net/http"
	"strings" // Нужен для HasPrefix

	"lingo-sprint/internal/api"
	"lingo-sprint/internal/database"

	// "github.com/golang-jwt/jwt/v5" // Оставим на случай, если захотим улучшить isLoggedIn
	"github.com/gorilla/mux"
)

// isLoggedIn проверяет простой cookie 'auth_status'.
func isLoggedIn(r *http.Request) bool {
	// Добавляем лог для отладки
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

	// --- ИНИЦИАЛИЗАЦИЯ РОУТЕРА ---
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
	// s.HandleFunc("/ai/explain-error", apiHandler.ExplainError).Methods("POST") // Оставляем закомментированным

	// --- 2. РЕГИСТРАЦИЯ СТРАНИЦ ПРИЛОЖЕНИЯ ---
	// Обработчик для /app - отдает index.html ТОЛЬКО если залогинен
	r.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		if !isLoggedIn(r) {
			log.Println("Redirecting /app to / (not logged in)") // Лог редиректа
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		log.Println("Serving /app (index.html)") // Лог отдачи
		http.ServeFile(w, r, "./web/static/index.html")
	}).Methods("GET")

    // Обработчики для /login и /register - просто редиректят на лендинг
    r.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request){
        log.Println("Redirecting /login to /") // Лог редиректа
        http.Redirect(w, r, "/", http.StatusSeeOther)
    }).Methods("GET")
    r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request){
         log.Println("Redirecting /register to /") // Лог редиректа
         http.Redirect(w, r, "/", http.StatusSeeOther)
    }).Methods("GET")

	// --- 3. РЕГИСТРАЦИЯ ЛЕНДИНГА ---
	// Обработчик для корневого пути "/"
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isLoggedIn(r) {
			log.Println("Redirecting / to /app (already logged in)") // Лог редиректа
			http.Redirect(w, r, "/app", http.StatusSeeOther)
			return
		}
		log.Println("Serving / (landing.html)") // Лог отдачи
		http.ServeFile(w, r, "./web/static/landing.html")
	}).Methods("GET")

	// --- 4. РЕГИСТРАЦИЯ FILESERVER ДЛЯ СТАТИКИ (CSS, JS, ИЗОБРАЖЕНИЯ) ---
	// Этот обработчик должен быть ПОСЛЕДНИМ ИЗ ОБЫЧНЫХ ПУТЕЙ
	fs := http.FileServer(http.Dir("./web/static/"))
	r.MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		path := r.URL.Path
		// Обслуживать через FileServer, если путь НЕ равен '/', '/app', '/login', '/register' И НЕ начинается с '/api'
		isStatic := path != "/" && path != "/app" && path != "/login" && path != "/register" && !strings.HasPrefix(path, "/api")
		// if isStatic { log.Printf("Serving static file: %s", path) } // Опциональный лог для статики
		return isStatic
	}).Handler(http.StripPrefix("/", fs))


	// --- ЗАПУСК СЕРВЕРА ---
	port := ":8080"
	log.Printf("Starting server on port %s", port)
	loggedRouter := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Логируем только реальные запросы, а не внутренние перенаправления
		// log.Printf("%s %s %s", req.RemoteAddr, req.Method, req.URL)
		r.ServeHTTP(w, req) // Передаем главному роутеру 'r'
	})
	if err := http.ListenAndServe(port, loggedRouter); err != nil {
		log.Fatalf("Server start error: %v", err)
	}
}