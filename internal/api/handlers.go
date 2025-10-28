package api

import (
	"bytes"
	// "context" // Keep for http.NewRequestWithContext
	"database/sql"
	"encoding/json"
	"errors"
	"fmt" // Keep for fmt.Sprintf
	"io"
	"log"
	"net/http" // Keep for http client/server
	"os" // Keep for os.Getenv
	"strconv"
	"time" // Keep for time.Second

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"lingo-sprint/internal/models"
)


// ApiHandler хранит подключение к базе данных
type ApiHandler struct {
	DB *sql.DB
}

// NewApiHandler создает новый обработчик с подключением к БД
func NewApiHandler(db *sql.DB) *ApiHandler {
	return &ApiHandler{DB: db}
}

// Credentials - структура для JSON-запросов регистрации/входа
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Claims - структура для данных внутри JWT-токена
type Claims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

// --- НОВАЯ ФУНКЦИЯ: RegisterUser ---
func (h *ApiHandler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Хэшируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Вставляем нового пользователя в БД
	_, err = h.DB.Exec("INSERT INTO users (email, password_hash) VALUES ($1, $2)", creds.Email, string(hashedPassword))
	if err != nil {
		// (Простая проверка, в реальном коде нужно проверять на 'duplicate key')
		respondWithError(w, http.StatusConflict, "Email already exists")
		return
	}

	respondWithJSON(w, http.StatusCreated, map[string]string{"message": "User registered successfully"})
}

// --- НОВАЯ ФУНКЦИЯ: LoginUser ---
func (h *ApiHandler) LoginUser(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Ищем пользователя по email
	var storedPasswordHash string
	var userID int
	err := h.DB.QueryRow("SELECT id, password_hash FROM users WHERE email = $1", creds.Email).Scan(&userID, &storedPasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusUnauthorized, "Invalid email or password")
		} else {
			respondWithError(w, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// Сравниваем хэш из БД с паролем из запроса
	err = bcrypt.CompareHashAndPassword([]byte(storedPasswordHash), []byte(creds.Password))
	if err != nil {
		// Пароль неверный
		respondWithError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// === Успех! Генерируем JWT-токен ===
	expirationTime := time.Now().Add(72 * time.Hour) // Токен "живет" 3 дня

	claims := &Claims{
		UserID: userID, // Кладем ID пользователя внутрь токена
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(JwtKey)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	// Отправляем токен клиенту
	respondWithJSON(w, http.StatusOK, map[string]string{"token": tokenString})
}


// SaveProgressRequest - структура для JSON-запроса о прогрессе
type SaveProgressRequest struct {
	SentenceID int  `json:"sentence_id"`
	IsCorrect  bool `json:"is_correct"`
}

// ExplainErrorRequest - structure for the AI explanation request
type ExplainErrorRequest struct {
    PromptRU     string `json:"prompt_ru"`     // Original Russian sentence
    CorrectEN    string `json:"correct_en"`    // Correct English answer
    UserAnswerEN string `json:"user_answer_en"` // User's incorrect answer
}

// --- ОБНОВЛЕННАЯ ФУНКЦИЯ: SaveProgress ("Один раз и готово") ---
func (h *ApiHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	// 1. Получаем ID пользователя
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok {
		log.Println("!!! SaveProgress ERROR: Could not get user ID from context")
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}
	log.Printf("SaveProgress: UserID=%d", userID)

	// 2. Читаем JSON
	var req SaveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("!!! SaveProgress ERROR: Could not decode JSON for UserID=%d: %v", userID, err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	log.Printf("SaveProgress: Received SentenceID=%d, IsCorrect=%t", req.SentenceID, req.IsCorrect)

	// --- УПРОЩЕННАЯ ЛОГИКА ---
	var nextStatus string
	var nextStreak int // Счетчик теперь не так важен для статуса, но сохраним его
	var nextReview time.Time
	now := time.Now()

	if req.IsCorrect {
		// ЛЮБОЙ ПРАВИЛЬНЫЙ ОТВЕТ -> Mastered
		nextStatus = "mastered"
		// Мы можем просто ставить счетчик 1 (или 5, не важно), т.к. статус уже mastered
		nextStreak = 1
		// Убираем надолго
		nextReview = now.Add(100 * 365 * 24 * time.Hour)
		log.Printf("SaveProgress Logic: CORRECT. Setting status to mastered.") // DEBUG LOG

	} else {
		// НЕПРАВИЛЬНО -> Learning, сброс счетчика
		nextStatus = "learning"
		nextStreak = 0 // Сбрасываем счетчик
		nextReview = now // Повторить как можно скорее
		log.Printf("SaveProgress Logic: INCORRECT. Setting status to learning, resetting streak.") // DEBUG LOG
	}
	// --------------------------

	// --- Обновляем или вставляем (UPSERT) ---
	sqlStatement := `
		INSERT INTO user_progress (user_id, sentence_id, status, correct_streak, next_review_date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, sentence_id)
		DO UPDATE SET
			status = EXCLUDED.status,
			correct_streak = EXCLUDED.correct_streak, -- Все еще обновляем счетчик для информации
			next_review_date = EXCLUDED.next_review_date;
	`
	log.Printf("SaveProgress: Executing UPSERT with UserID=%d, SentenceID=%d, Status=%s, Streak=%d, ReviewDate=%v", userID, req.SentenceID, nextStatus, nextStreak, nextReview)
	result, err := h.DB.Exec(sqlStatement, userID, req.SentenceID, nextStatus, nextStreak, nextReview)
	if err != nil {
		log.Printf("!!! SaveProgress ERROR: UPSERT failed for UserID=%d, SentenceID=%d: %v", userID, req.SentenceID, err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save progress")
		return
	}
	rowsAffected, _ := result.RowsAffected()
	log.Printf("SaveProgress: UPSERT successful for UserID=%d, SentenceID=%d. Rows affected: %d", userID, req.SentenceID, rowsAffected)

	respondWithJSON(w, http.StatusOK, map[string]string{"message": "Progress saved"})
}


// --- Существующие функции (без изменений) ---

func (h *ApiHandler) GetLevels(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query("SELECT id, title FROM levels ORDER BY title")
	if err != nil {
		http.Error(w, "Failed to query levels", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	levels := []models.Level{}
	for rows.Next() {
		var l models.Level
		if err := rows.Scan(&l.ID, &l.Title); err != nil {
			log.Printf("Error scanning level: %v", err)
			continue
		}
		levels = append(levels, l)
	}

	respondWithJSON(w, http.StatusOK, levels)
}

func (h *ApiHandler) GetLessonsByLevel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	levelID, err := strconv.Atoi(vars["level_id"])
	if err != nil {
		http.Error(w, "Invalid level ID", http.StatusBadRequest)
		return
	}

	rows, err := h.DB.Query("SELECT id, level_id, lesson_number, title FROM lessons WHERE level_id = $1 ORDER BY lesson_number", levelID)
	if err != nil {
		http.Error(w, "Failed to query lessons", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	lessons := []models.Lesson{}
	for rows.Next() {
		var l models.Lesson
		if err := rows.Scan(&l.ID, &l.LevelID, &l.LessonNumber, &l.Title); err != nil {
			log.Printf("Error scanning lesson: %v", err)
			continue
		}
		lessons = append(lessons, l)
	}

	respondWithJSON(w, http.StatusOK, lessons)
}

// --- ОБНОВЛЕННАЯ ФУНКЦИЯ: GetSentencesByLesson ---
func (h *ApiHandler) GetSentencesByLesson(w http.ResponseWriter, r *http.Request) {
	// 1. Получаем ID пользователя из "контекста"
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}

	// 2. Получаем lesson_id из URL
	vars := mux.Vars(r)
	lessonID, err := strconv.Atoi(vars["lesson_id"])
	if err != nil {
		http.Error(w, "Invalid lesson ID", http.StatusBadRequest)
		return
	}

	// 3. === НОВЫЙ SQL-ЗАПРОС с LEFT JOIN ===
	// Мы "присоединяем" прогресс пользователя к каждому предложению.
	// Если прогресса нет, 'up.status' и 'up.correct_streak' будут NULL.
	sqlQuery := `
		SELECT 
			s.id, s.lesson_id, s.order_number, s.prompt_ru, s.answer_en, s.transcription, s.audio_path,
			up.status, up.correct_streak 
		FROM sentences s
		LEFT JOIN user_progress up 
			ON s.id = up.sentence_id AND up.user_id = $1
		WHERE s.lesson_id = $2
		ORDER BY s.order_number;
	`

	rows, err := h.DB.Query(sqlQuery, userID, lessonID)
	if err != nil {
		log.Printf("Failed to query sentences with progress: %v", err)
		http.Error(w, "Failed to query sentences", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sentences := []models.Sentence{}
	for rows.Next() {
		var s models.Sentence
		// 4. Сканируем новые nullable поля (s.Status, s.CorrectStreak)
		if err := rows.Scan(
			&s.ID, &s.LessonID, &s.OrderNumber, &s.PromptRU, &s.AnswerEN, 
			&s.Transcription, &s.AudioPath,
			&s.Status, &s.CorrectStreak, // <-- Новые поля
		); err != nil {
			log.Printf("Error scanning sentence with progress: %v", err)
			continue
		}
		sentences = append(sentences, s)
	}

	respondWithJSON(w, http.StatusOK, sentences)
}


// --- НОВАЯ AI ФУНКЦИЯ: ExplainError ---
// --- НОВАЯ AI ФУНКЦИЯ: ExplainError (с прямым HTTP запросом) ---
func (h *ApiHandler) ExplainError(w http.ResponseWriter, r *http.Request) {
	// 1. Read the request body
	var req ExplainErrorRequest // Use 'req' for the request body struct
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// 2. Get Hugging Face token from environment
	hfToken := os.Getenv("HUGGINGFACE_TOKEN") // <<< RESTORED
	if hfToken == "" {
		log.Println("HUGGINGFACE_TOKEN environment variable is not set!")
		respondWithError(w, http.StatusInternalServerError, "AI service configuration error")
		return
	}

	// === DIRECT HTTP REQUEST ===

	// Use a known, simple model for testing
	// model := "google/flan-t5-small"
	model := "gpt2" // Use the classic GPT-2 model
	apiURL := fmt.Sprintf("https://api-inference.huggingface.co/models/%s", model)

	// Construct the prompt (same as before)
	prompt := fmt.Sprintf(`You are a helpful English tutor explaining a mistake to a student learning English.
The student was asked to translate the Russian sentence: "%s"
The correct English translation is: "%s"
The student answered: "%s"

Explain the student's mistake clearly and concisely in Russian. Focus only on the main error (grammar, vocabulary, or spelling). Be encouraging.`,
		req.PromptRU, req.CorrectEN, req.UserAnswerEN) // <<< RESTORED (Uses the 'req' struct fields)

	// Prepare the request payload for HF API
	payload := map[string]interface{}{
		"inputs": prompt,
		"parameters": map[string]interface{}{
			 "max_new_tokens": 150,
			//  "temperature": 0.7,
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal HF payload: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI request preparation failed")
		return
	}

	// Create the HTTP request (Use r.Context() from the incoming request)
	hfReq, err := http.NewRequestWithContext(r.Context(), "POST", apiURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Failed to create HF request: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI request creation failed")
		return
	}

	// Add Headers (Authorization and Content-Type)
	hfReq.Header.Set("Authorization", "Bearer "+hfToken) // <<< Uses hfToken (now defined)
	hfReq.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{Timeout: 30 * time.Second}
	log.Printf("Sending direct request to: %s", apiURL)
	hfResp, err := client.Do(hfReq)
	if err != nil {
		log.Printf("Direct HF API call failed (network/timeout): %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI service connection error")
		return
	}
	defer hfResp.Body.Close()

	// Read the response body
	bodyBytes, err := io.ReadAll(hfResp.Body)
	if err != nil {
		log.Printf("Failed to read HF response body: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI response reading failed")
		return
	}
	bodyString := string(bodyBytes)

	log.Printf("Direct HF Response Status: %s", hfResp.Status)
	log.Printf("Direct HF Response Body: %s", bodyString)

	// Check the status code FROM HUGGING FACE
	if hfResp.StatusCode != http.StatusOK {
		log.Printf("Hugging Face returned non-OK status: %d", hfResp.StatusCode)
        // Pass the raw body back to the frontend for debugging if it's not 200 OK
		respondWithError(w, hfResp.StatusCode, fmt.Sprintf("AI service error (%d): %s", hfResp.StatusCode, bodyString))
		return
	}

	// --- Try to parse the response ---
	var result []map[string]string
	err = json.Unmarshal(bodyBytes, &result)
	var completion string
	if err == nil && len(result) > 0 && result[0]["generated_text"] != "" {
		completion = result[0]["generated_text"]
	} else {
		log.Printf("Could not parse HF response JSON, returning raw body. Parse error: %v", err)
		completion = bodyString
	}
	// =============================

	// 6. Return the explanation
	respondWithJSON(w, http.StatusOK, map[string]string{"explanation": completion})
}

// --- Вспомогательные функции ---

// respondWithJSON - вспомогательная функция для отправки JSON-ответов
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// --- НОВАЯ ФУНКЦИЯ: respondWithError ---
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}