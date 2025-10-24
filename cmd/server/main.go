package main

import (
	"fmt"
	"log"
	"net/http"
	"lingo-sprint/internal/database" // Мы добавим это чуть позже
)

func main() {
	// Шаг 2: Подключение к базе данных. Пока закомментировано.
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("Could not connect to the database: %v", err)
	}
	log.Println("Successfully connected to the database!")
	defer db.Close() // Закрываем соединение при завершении работы

	// Создаем простой обработчик для главной страницы
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, Lingo-Sprint is running!")
	})

	// Запускаем сервер на порту 8080, как указано в docker-compose.yml
	port := ":8080"
	log.Printf("Starting server on port %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}