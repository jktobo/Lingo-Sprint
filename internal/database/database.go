package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time" // <-- Добавили 'time'

	_ "github.com/jackc/pgx/v5/stdlib" // Анонимный импорт PostgreSQL драйвера
)

// Connect подключается к базе данных, используя DATABASE_URL из окружения.
func Connect() (*sql.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	// sql.Open не устанавливает соединение, а только готовит пул
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection (driver error): %w", err)
	}

	// === НОВАЯ ЛОГИКА ПОВТОРНЫХ ПОПЫТОК ===
	maxRetries := 10
	var pingErr error

	for i := 1; i <= maxRetries; i++ {
		pingErr = db.Ping()
		if pingErr == nil {
			// Успех!
			return db, nil
		}

		log.Printf("DB not ready (attempt %d/%d). Retrying in 3 seconds...", i, maxRetries)
		time.Sleep(3 * time.Second)
	}
	// === КОНЕЦ НОВОЙ ЛОГИКИ ===

	// Если мы вышли из цикла, значит, подключиться не удалось
	db.Close() // Очищаем пул
	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, pingErr)
}