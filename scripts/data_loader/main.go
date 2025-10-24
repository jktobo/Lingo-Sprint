package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Подключаемся к БД "снаружи" Docker
const dbURL = "postgres://lingo_user:supersecretpassword@localhost:5433/lingo_db?sslmode=disable"
const csvDir = "scripts/"

// Регулярное выражение для парсинга имени файла: (A0)_lesson_(1).csv
var fileNameRegex = regexp.MustCompile(`^([A-Z0-9]+)_lesson_(\d+)\.csv$`)

func main() {
	log.Println("Загрузчик данных (полная версия) запущен...")

	// --- 1. Подключение к БД ---
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Не удалось подключиться к базе данных: %v", err)
	}
	defer conn.Close(ctx)
	log.Println("Успешно подключен к БД (localhost:5433)")

	// --- 2. Получаем все CSV файлы из папки ---
	files, err := filepath.Glob(filepath.Join(csvDir, "*.csv"))
	if err != nil {
		log.Fatalf("Не удалось найти CSV файлы: %v", err)
	}

	if len(files) == 0 {
		log.Fatal("В папке /scripts не найдено ни одного .csv файла.")
	}

	log.Printf("Найдено %d CSV файлов. Начинаю обработку...", len(files))

	// --- 3. Обрабатываем каждый файл ---
	totalSentencesLoaded := 0
	for _, filePath := range files {
		fileName := filepath.Base(filePath)

		// 3.1. Парсим имя файла, чтобы узнать уровень и номер урока
		matches := fileNameRegex.FindStringSubmatch(fileName)
		if len(matches) != 3 {
			log.Printf("ПРЕДУПРЕЖДЕНИЕ: Файл '%s' имеет некорректное имя. Пропускаю.", fileName)
			continue
		}

		levelTitle := strings.ToUpper(matches[1])
		lessonNumber, _ := strconv.Atoi(matches[2])

		log.Printf("--- Обработка: %s (Уровень: %s, Урок: %d) ---", fileName, levelTitle, lessonNumber)

		// 3.2. Получаем ID уровня из БД
		var levelID int
		err = conn.QueryRow(ctx, "SELECT id FROM levels WHERE title = $1", levelTitle).Scan(&levelID)
		if err != nil {
			log.Printf("ОШИБКА: Уровень '%s' не найден в БД. Пропускаю файл. (Проверьте init.sql)", levelTitle)
			continue
		}

		// 3.3. Создаем урок и получаем его ID
		var lessonID int
		lessonTitle := fmt.Sprintf("Урок %d (%s)", lessonNumber, levelTitle)
		err = conn.QueryRow(ctx,
			"INSERT INTO lessons (level_id, lesson_number, title) VALUES ($1, $2, $3) RETURNING id",
			levelID, lessonNumber, lessonTitle,
		).Scan(&lessonID)
		if err != nil {
			log.Printf("ОШИБКА: Не удалось создать урок %d: %v. Возможно, он уже существует.", lessonNumber, err)
			continue
		}

		// 3.4. Читаем CSV
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("ОШИБКА: Не удалось открыть CSV файл '%s': %v", filePath, err)
			continue
		}

		reader := csv.NewReader(file)
		records, err := reader.ReadAll()
		file.Close()
		if err != nil {
			log.Printf("ОШИБКА: Не удалось прочитать CSV '%s': %v", filePath, err)
			continue
		}

		// 3.5. Готовим пакетную вставку (Batch Insert)
		batch := &pgx.Batch{}
		sqlStatement := `
			INSERT INTO sentences (lesson_id, order_number, prompt_ru, answer_en, transcription, audio_path)
			VALUES ($1, $2, $3, $4, $5, $6)
		`
		
		sentencesInThisFile := 0
		for _, record := range records {
			if len(record) < 5 {
				log.Printf("ПРЕДУПРЕЖДЕНИЕ: В файле %s найдена некорректная строка, пропускаю.", fileName)
				continue
			}
			
			orderNum, err := strconv.Atoi(record[0])
			if err != nil {
				log.Printf("ПРЕДУПРЕЖДЕНИЕ: В файле %s некорректный ID '%s', пропускаю.", fileName, record[0])
				continue
			}

			batch.Queue(sqlStatement,
				lessonID,  // $1
				orderNum,  // $2 (record[0])
				record[1], // $3 (prompt_ru)
				record[2], // $4 (answer_en)
				record[3], // $5 (transcription)
				record[4], // $6 (audio_path)
			)
			sentencesInThisFile++
		}

		// 3.6. Выполняем пакетную вставку
		br := conn.SendBatch(ctx, batch)
		_, err = br.Exec()
		if err != nil {
			log.Printf("ОШИБКА: Пакетная вставка для урока %d не удалась: %v", lessonNumber, err)
			continue
		}
		br.Close()

		log.Printf("Успешно загружено %d предложений для Урока %d.", sentencesInThisFile, lessonNumber)
		totalSentencesLoaded += sentencesInThisFile
	}

	log.Printf("--- ЗАВЕРШЕНО ---")
	log.Printf("Всего файлов обработано: %d", len(files))
	log.Printf("Всего предложений загружено в БД: %d", totalSentencesLoaded)
}