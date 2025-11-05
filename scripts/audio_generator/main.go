package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync" // Для параллельной обработки
	"time" // <-- ДОБАВИТЬ ЭТУ СТРОКУ

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib" // Ваш драйвер БД
	texttospeechpb "google.golang.org/genproto/googleapis/cloud/texttospeech/v1"
)

// Sentence - упрощенная структура для нашей задачи
type Sentence struct {
	ID       int
	AnswerEN string
}

// === Настройки ===
const outputDir = "media"          // Папка для сохранения .mp3
const maxWorkers = 10              // Кол-во одновременных запросов к Google
// =================

func main() {
	log.Println("Запуск генератора аудио...")

	// 1. Загружаем .env (из корня проекта)
	// Мы ожидаем, что скрипт запускается из корня: go run ./scripts/audio_generator/main.go
	if err := godotenv.Load(); err != nil {
		log.Fatal("Ошибка загрузки .env файла. Убедитесь, что он в корне:", err)
	}

	// 2. Подключаемся к БД (используя DATABASE_URL из .env)
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

	// 3. Создаем TTS клиент
	// (Он автоматически найдет ключ через GOOGLE_APPLICATION_CREDENTIALS)
	ctx := context.Background()
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		log.Fatalf("Не удалось создать TTS клиент: %v", err)
	}
	defer client.Close()
	log.Println("Успешно подключен к Google TTS API.")

	// 4. Создаем папку 'media', если ее нет
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		log.Fatalf("Не удалось создать папку %s: %v", outputDir, err)
	}

	// 5. Получаем все предложения, которые нужно озвучить
	sentences, err := getSentencesToProcess(db)
	if err != nil {
		log.Fatalf("Ошибка получения предложений: %v", err)
	}

	if len(sentences) == 0 {
		log.Println("Все предложения уже озвучены. Завершение.")
		return
	}
	log.Printf("Найдено %d предложений для озвучки.", len(sentences))

	// 6. Запускаем пул воркеров для обработки
	jobs := make(chan Sentence, len(sentences))
	results := make(chan string, len(sentences))
	var wg sync.WaitGroup

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, client, db, jobs, results)
	}

	// 7. Отправляем задания
	for _, s := range sentences {
		jobs <- s
	}
	close(jobs)

	// 8. Ждем завершения
	wg.Wait()
	close(results)

	log.Println("--- Генерация завершена! ---")
	processedCount := 0
	for msg := range results {
		log.Println(msg)
		processedCount++
	}
	log.Printf("Успешно обработано: %d", processedCount)
}

// getSentencesToProcess получает все предложения, где audio_path пуст
func getSentencesToProcess(db *sql.DB) ([]Sentence, error) {
	rows, err := db.Query("SELECT id, answer_en FROM sentences WHERE audio_path IS NULL OR audio_path = ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sentences []Sentence
	for rows.Next() {
		var s Sentence
		if err := rows.Scan(&s.ID, &s.AnswerEN); err != nil {
			log.Printf("Ошибка сканирования строки: %v", err)
			continue
		}
		sentences = append(sentences, s)
	}
	return sentences, nil
}

// worker - это один "рабочий", который берет задания из канала jobs
func worker(ctx context.Context, wg *sync.WaitGroup, client *texttospeech.Client, db *sql.DB, jobs <-chan Sentence, results chan<- string) {
	defer wg.Done()

	for s := range jobs {
		filePath := filepath.Join(outputDir, fmt.Sprintf("%d.mp3", s.ID))
		
		// 1. Генерируем аудио
		err := synthesizeAndSave(ctx, client, s.AnswerEN, filePath)
		if err != nil {
			log.Printf("Ошибка (ID %d): Не удалось сгенерировать: %v", s.ID, err)
			continue
		}

		// 2. Обновляем путь в БД
		dbPath := fmt.Sprintf("media/%d.mp3", s.ID)
		_, err = db.Exec("UPDATE sentences SET audio_path = $1 WHERE id = $2", dbPath, s.ID)
		if err != nil {
			log.Printf("Ошибка (ID %d): Не удалось обновить БД: %v", s.ID, err)
			continue
		}

		results <- fmt.Sprintf("Успех: ID %d -> %s", s.ID, dbPath)

		// --- НОВОЕ: ПАУЗА ---
		// Ждем 700мс, чтобы не превысить лимит (1000/мин)
		// (10 воркеров * (1000ms / 700ms) = ~14 req/sec = ~850 req/min)
		time.Sleep(700 * time.Millisecond)
	}
}

// synthesizeAndSave вызывает Google API и сохраняет .mp3 файл
func synthesizeAndSave(ctx context.Context, client *texttospeech.Client, text, outputPath string) error {
	req := &texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		// --- ГОЛОС (en-US, Standard, 'F') ---
		// Вы можете выбрать любой другой "Standard" (не "Wavenet") голос
		// для бесплатного лимита.
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "en-US",
			SsmlGender:   texttospeechpb.SsmlVoiceGender_FEMALE,
			Name:         "en-US-Standard-F",
		},
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		},
	}

	resp, err := client.SynthesizeSpeech(ctx, req)
	if err != nil {
		return fmt.Errorf("SynthesizeSpeech: %v", err)
	}

	// Записываем полученные байты в .mp3 файл
	err = ioutil.WriteFile(outputPath, resp.AudioContent, 0644)
	if err != nil {
		return fmt.Errorf("WriteFile: %v", err)
	}

	return nil
}