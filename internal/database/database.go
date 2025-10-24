package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib" // Анонимный импорт PostgreSQL драйвера
)

// Connect подключается к базе данных, используя DATABASE_URL из окружения.
func Connect() (*sql.DB, error) {
	// Получаем URL для подключения из переменной окружения,
	// которую мы задали в docker-compose.yml
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	// Открываем соединение с базой данных
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Проверяем, что соединение действительно установлено
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}