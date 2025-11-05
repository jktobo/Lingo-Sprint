package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib" // Ваш драйвер БД
)

const scriptsDir = "scripts" // Папка, где лежат CSV

// Структура для сортировки файлов
type lessonFile struct {
	Path        string
	LevelName   string // "A0", "A1"
	LessonNum   int    // 1, 2, 10
	LessonTitle string // "A0 Lesson 1"
}

func main() {
	log.Println("Запуск загрузчика данных...")
	startTime := time.Now()

	// 1. Загружаем .env (из корня проекта)
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Ошибка загрузки .env файла: %v", err)
	}

	// 2. Подключаемся к БД
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL не найден в .env")
	}
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("Не удалось подключиться к БД: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("БД недоступна: %v", err)
	}
	log.Println("Успешно подключен к БД.")

	// 3. Начинаем транзакцию
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("Не удалось начать транзакцию: %v", err)
	}
	// Если что-то пойдет не так, откатываем изменения
	defer tx.Rollback()

	// 4. Главная логика
	if err := processData(tx); err != nil {
		log.Fatalf("Ошибка обработки данных: %v. \n--- ИЗМЕНЕНИЯ ОТКАТЫВАЮТСЯ ---", err)
	}

	// 5. Если все прошло успешно, сохраняем изменения
	if err := tx.Commit(); err != nil {
		log.Fatalf("Не удалось сохранить транзакцию: %v", err)
	}

	log.Printf("--- УСПЕХ! --- \nДанные успешно загружены за %v.", time.Since(startTime))
}

func processData(tx *sql.Tx) error {
	// 1. Найти и отсортировать все CSV файлы
	log.Println("Поиск и сортировка CSV файлов...")
	lessonFiles, err := findAndSortFiles(scriptsDir)
	if err != nil {
		return err
	}
	log.Printf("Найдено %d файлов уроков для обработки.", len(lessonFiles))

	// 2. Обработать каждый файл
	totalSentences := 0
	for _, lf := range lessonFiles {
		log.Printf("Обработка: %s (Уровень: %s, Урок: %d)", filepath.Base(lf.Path), lf.LevelName, lf.LessonNum)

		// 2a. Получить (или создать) ID уровня
		levelID, err := getOrInsertLevel(tx, lf.LevelName)
		if err != nil {
			return fmt.Errorf("ошибка уровня %s: %v", lf.LevelName, err)
		}

		// 2b. Получить (или создать) ID урока
		lessonID, err := getOrInsertLesson(tx, levelID, lf.LessonNum, lf.LessonTitle)
		if err != nil {
			return fmt.Errorf("ошибка урока %d (уровень %d): %v", lf.LessonNum, levelID, err)
		}

		// 2c. Загрузить предложения из CSV
		count, err := loadSentences(tx, lessonID, lf.Path)
		if err != nil {
			return fmt.Errorf("ошибка загрузки %s: %v", lf.Path, err)
		}
		totalSentences += count
	}

	log.Printf("Загрузка завершена. Всего обработано предложений: %d", totalSentences)
	return nil
}

// findAndSortFiles находит, парсит и сортирует CSV
func findAndSortFiles(dir string) ([]lessonFile, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var lessonFiles []lessonFile
	// Регулярное выражение для парсинга имени файла
	// (A0|A1|A2|B1|B2)_lesson_([0-9]+).csv
	r := regexp.MustCompile(`^(A0|A1|A2|B1|B2)_lesson_([0-9]+)\.csv$`)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		matches := r.FindStringSubmatch(fileName)

		// 0: A0_lesson_1.csv
		// 1: A0
		// 2: 1
		if len(matches) == 3 {
			levelName := matches[1]
			lessonNum, _ := strconv.Atoi(matches[2]) // Ошибка Atoi здесь маловероятна
			title := fmt.Sprintf("%s - Урок %d", levelName, lessonNum)

			lessonFiles = append(lessonFiles, lessonFile{
				Path:        filepath.Join(dir, fileName),
				LevelName:   levelName,
				LessonNum:   lessonNum,
				LessonTitle: title,
			})
		}
	}

	// Сортируем! Сначала по имени уровня (A0, A1), потом по номеру урока (1, 2, 10)
	sort.Slice(lessonFiles, func(i, j int) bool {
		if lessonFiles[i].LevelName != lessonFiles[j].LevelName {
			return lessonFiles[i].LevelName < lessonFiles[j].LevelName
		}
		return lessonFiles[i].LessonNum < lessonFiles[j].LessonNum
	})

	return lessonFiles, nil
}

// getOrInsertLevel находит ID уровня или создает новый
func getOrInsertLevel(tx *sql.Tx, title string) (int, error) {
	var id int
	err := tx.QueryRow("SELECT id FROM levels WHERE title = $1", title).Scan(&id)
	if err == sql.ErrNoRows {
		// Не найден, создаем
		err = tx.QueryRow("INSERT INTO levels (title) VALUES ($1) RETURNING id", title).Scan(&id)
		if err != nil {
			return 0, err
		}
		log.Printf(" -> Создан новый уровень: '%s' (ID: %d)", title, id)
	} else if err != nil {
		// Другая ошибка
		return 0, err
	}
	// Найден
	return id, nil
}

// getOrInsertLesson находит ID урока или создает новый
func getOrInsertLesson(tx *sql.Tx, levelID int, lessonNum int, title string) (int, error) {
	var id int
	err := tx.QueryRow("SELECT id FROM lessons WHERE level_id = $1 AND lesson_number = $2", levelID, lessonNum).Scan(&id)
	if err == sql.ErrNoRows {
		// Не найден, создаем
		err = tx.QueryRow("INSERT INTO lessons (level_id, lesson_number, title) VALUES ($1, $2, $3) RETURNING id",
			levelID, lessonNum, title).Scan(&id)
		if err != nil {
			return 0, err
		}
		log.Printf("   -> Создан новый урок: '%s' (ID: %d)", title, id)
	} else if err != nil {
		// Другая ошибка
		return 0, err
	}
	// Найден
	return id, nil
}

// loadSentences читает CSV и вставляет предложения
// loadSentences читает CSV и вставляет/обновляет предложения
func loadSentences(tx *sql.Tx, lessonID int, csvPath string) (int, error) {
	file, err := os.Open(csvPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	order := 1 // Порядковый номер предложения в уроке
	count := 0
	
	// Этот запрос ОБНОВИТ существующие строки, сохранив их ID
	// Он также обнуляет audio_path, чтобы пометить их для новой озвучки
	sqlStatement := `
		INSERT INTO sentences (lesson_id, order_number, prompt_ru, answer_en, transcription, audio_path) 
		VALUES ($1, $2, $3, $4, $5, NULL) 
		ON CONFLICT (lesson_id, order_number) 
		DO UPDATE SET
			prompt_ru = EXCLUDED.prompt_ru,
			answer_en = EXCLUDED.answer_en,
			transcription = EXCLUDED.transcription,
			audio_path = NULL;` // Обнуляем аудио, т.к. текст 100% изменился

	stmt, err := tx.PrepareContext(context.Background(), sqlStatement)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()


	// Пропускаем заголовок (headers)
	_, err = reader.Read()
	if err == io.EOF { 
		return 0, nil // Файл пустой (только заголовок)
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break // Файл закончился
		}
		if err != nil {
			return count, err
		}

		// === ВАЖНОЕ ИСПРАВЛЕНИЕ ЗДЕСЬ ===
		// Наш CSV: sentence_id(0), prompt_ru(1), answer_en(2), transcription(3)
		
		if len(record) < 3 { // Нам нужны как минимум колонки 0, 1, 2
			log.Printf("   ! Пропуск строки (мало столбцов): %v", record)
			continue
		}

		promptRU := record[1] // <-- Берем ИНДЕКС 1
		answerEN := record[2] // <-- Берем ИНДЕКС 2
		
		transcription := ""
		if len(record) > 3 {
			transcription = strings.TrimSpace(record[3]) // <-- Берем ИНДЕКС 3
		}
		// ================================

		// Вставляем или Обновляем в БД
		_, err = stmt.ExecContext(context.Background(), lessonID, order, promptRU, answerEN, transcription)
		if err != nil {
			return count, err
		}
		
		order++
		count++
	}
	return count, nil
}






// package main

// import (
// 	"context"
// 	"encoding/csv"
// 	"fmt"
// 	"log"
// 	"os"
// 	"path/filepath"
// 	"regexp"
// 	"strconv"
// 	"strings"

// 	"github.com/jackc/pgx/v5"
// )

// // Подключаемся к БД "снаружи" Docker
// const dbURL = "postgres://lingo_user:supersecretpassword@localhost:5433/lingo_db?sslmode=disable"
// const csvDir = "scripts/"

// // Регулярное выражение для парсинга имени файла: (A0)_lesson_(1).csv
// var fileNameRegex = regexp.MustCompile(`^([A-Z0-9]+)_lesson_(\d+)\.csv$`)

// func main() {
// 	log.Println("Загрузчик данных (полная версия) запущен...")

// 	// --- 1. Подключение к БД ---
// 	ctx := context.Background()
// 	conn, err := pgx.Connect(ctx, dbURL)
// 	if err != nil {
// 		log.Fatalf("Не удалось подключиться к базе данных: %v", err)
// 	}
// 	defer conn.Close(ctx)
// 	log.Println("Успешно подключен к БД (localhost:5433)")

// 	// --- 2. Получаем все CSV файлы из папки ---
// 	files, err := filepath.Glob(filepath.Join(csvDir, "*.csv"))
// 	if err != nil {
// 		log.Fatalf("Не удалось найти CSV файлы: %v", err)
// 	}

// 	if len(files) == 0 {
// 		log.Fatal("В папке /scripts не найдено ни одного .csv файла.")
// 	}

// 	log.Printf("Найдено %d CSV файлов. Начинаю обработку...", len(files))

// 	// --- 3. Обрабатываем каждый файл ---
// 	totalSentencesLoaded := 0
// 	for _, filePath := range files {
// 		fileName := filepath.Base(filePath)

// 		// 3.1. Парсим имя файла, чтобы узнать уровень и номер урока
// 		matches := fileNameRegex.FindStringSubmatch(fileName)
// 		if len(matches) != 3 {
// 			log.Printf("ПРЕДУПРЕЖДЕНИЕ: Файл '%s' имеет некорректное имя. Пропускаю.", fileName)
// 			continue
// 		}

// 		levelTitle := strings.ToUpper(matches[1])
// 		lessonNumber, _ := strconv.Atoi(matches[2])

// 		log.Printf("--- Обработка: %s (Уровень: %s, Урок: %d) ---", fileName, levelTitle, lessonNumber)

// 		// 3.2. Получаем ID уровня из БД
// 		var levelID int
// 		err = conn.QueryRow(ctx, "SELECT id FROM levels WHERE title = $1", levelTitle).Scan(&levelID)
// 		if err != nil {
// 			log.Printf("ОШИБКА: Уровень '%s' не найден в БД. Пропускаю файл. (Проверьте init.sql)", levelTitle)
// 			continue
// 		}

// 		// 3.3. Создаем урок и получаем его ID
// 		var lessonID int
// 		lessonTitle := fmt.Sprintf("Урок %d (%s)", lessonNumber, levelTitle)
// 		err = conn.QueryRow(ctx,
// 			"INSERT INTO lessons (level_id, lesson_number, title) VALUES ($1, $2, $3) RETURNING id",
// 			levelID, lessonNumber, lessonTitle,
// 		).Scan(&lessonID)
// 		if err != nil {
// 			log.Printf("ОШИБКА: Не удалось создать урок %d: %v. Возможно, он уже существует.", lessonNumber, err)
// 			continue
// 		}

// 		// 3.4. Читаем CSV
// 		file, err := os.Open(filePath)
// 		if err != nil {
// 			log.Printf("ОШИБКА: Не удалось открыть CSV файл '%s': %v", filePath, err)
// 			continue
// 		}

// 		reader := csv.NewReader(file)
// 		records, err := reader.ReadAll()
// 		file.Close()
// 		if err != nil {
// 			log.Printf("ОШИБКА: Не удалось прочитать CSV '%s': %v", filePath, err)
// 			continue
// 		}

// 		// 3.5. Готовим пакетную вставку (Batch Insert)
// 		batch := &pgx.Batch{}
// 		sqlStatement := `
// 			INSERT INTO sentences (lesson_id, order_number, prompt_ru, answer_en, transcription, audio_path)
// 			VALUES ($1, $2, $3, $4, $5, $6)
// 		`
		
// 		sentencesInThisFile := 0
// 		for _, record := range records {
// 			if len(record) < 5 {
// 				log.Printf("ПРЕДУПРЕЖДЕНИЕ: В файле %s найдена некорректная строка, пропускаю.", fileName)
// 				continue
// 			}
			
// 			orderNum, err := strconv.Atoi(record[0])
// 			if err != nil {
// 				log.Printf("ПРЕДУПРЕЖДЕНИЕ: В файле %s некорректный ID '%s', пропускаю.", fileName, record[0])
// 				continue
// 			}

// 			batch.Queue(sqlStatement,
// 				lessonID,  // $1
// 				orderNum,  // $2 (record[0])
// 				record[1], // $3 (prompt_ru)
// 				record[2], // $4 (answer_en)
// 				record[3], // $5 (transcription)
// 				record[4], // $6 (audio_path)
// 			)
// 			sentencesInThisFile++
// 		}

// 		// 3.6. Выполняем пакетную вставку
// 		br := conn.SendBatch(ctx, batch)
// 		_, err = br.Exec()
// 		if err != nil {
// 			log.Printf("ОШИБКА: Пакетная вставка для урока %d не удалась: %v", lessonNumber, err)
// 			continue
// 		}
// 		br.Close()

// 		log.Printf("Успешно загружено %d предложений для Урока %d.", sentencesInThisFile, lessonNumber)
// 		totalSentencesLoaded += sentencesInThisFile
// 	}

// 	log.Printf("--- ЗАВЕРШЕНО ---")
// 	log.Printf("Всего файлов обработано: %d", len(files))
// 	log.Printf("Всего предложений загружено в БД: %d", totalSentencesLoaded)
// }