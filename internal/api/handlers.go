package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

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
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}
	_, err = h.DB.Exec("INSERT INTO users (email, password_hash) VALUES ($1, $2)", creds.Email, string(hashedPassword))
	if err != nil {
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
	err = bcrypt.CompareHashAndPassword([]byte(storedPasswordHash), []byte(creds.Password))
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}
	expirationTime := time.Now().Add(72 * time.Hour)
	claims := &Claims{
		UserID: userID,
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
	respondWithJSON(w, http.StatusOK, map[string]string{"token": tokenString})
}


// --- Structs ---
type SaveProgressRequest struct {
	SentenceID int  `json:"sentence_id"`
	IsCorrect  bool `json:"is_correct"`
}
type ExplainErrorRequest struct {
	PromptRU     string `json:"prompt_ru"`
	CorrectEN    string `json:"correct_en"`
	UserAnswerEN string `json:"user_answer_en"`
}
// Структуры для AI
type hfMessage struct { Role string `json:"role"`; Content string `json:"content"` }
type hfChatRequest struct { Model string `json:"model"`; Messages []hfMessage `json:"messages"`; Stream bool `json:"stream"` }
type hfChatResponse struct { Choices []struct { Message struct { Role string `json:"role"`; Content string `json:"content"` } `json:"message"` } `json:"choices"`; Error *struct { Message string `json:"message"` } `json:"error,omitempty"` }


// --- SaveProgress ( ▼▼▼ ОБНОВЛЕННАЯ ЛОГИКА ▼▼▼ ) ---
func (h *ApiHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}

	var req SaveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// 1. Определяем параметры для user_progress
	var nextStatus string
	var nextStreak int
	var nextReview time.Time
	now := time.Now()

	// Для обновления статистики пользователя
	incrementCorrect := 0

	if req.IsCorrect {
		nextStatus = "mastered"
		nextStreak = 1
		nextReview = now.Add(100 * 365 * 24 * time.Hour)
		incrementCorrect = 1 // +1 к правильным ответам
	} else {
		nextStatus = "learning"
		nextStreak = 0
		nextReview = now
		incrementCorrect = 0 // +0 к правильным
	}

	// 2. Обновляем прогресс по предложению (как и раньше)
	sqlProgress := `
		INSERT INTO user_progress (user_id, sentence_id, status, correct_streak, next_review_date, updated_at) 
		VALUES ($1, $2, $3, $4, $5, NOW()) 
		ON CONFLICT (user_id, sentence_id) 
		DO UPDATE SET 
			status = EXCLUDED.status, 
			correct_streak = EXCLUDED.correct_streak, 
			next_review_date = EXCLUDED.next_review_date,
			updated_at = NOW();
	`
	_, err := h.DB.Exec(sqlProgress, userID, req.SentenceID, nextStatus, nextStreak, nextReview)
	if err != nil {
		log.Printf("Error saving sentence progress: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save sentence progress")
		return
	}

	// 3. ▼▼▼ ОБНОВЛЯЕМ ГЛОБАЛЬНУЮ СТАТИСТИКУ ПОЛЬЗОВАТЕЛЯ ▼▼▼
	// total_attempts всегда +1
	// total_correct увеличиваем на 1 или 0
	sqlStats := `UPDATE users SET total_attempts = total_attempts + 1, total_correct = total_correct + $1 WHERE id = $2`
	_, err = h.DB.Exec(sqlStats, incrementCorrect, userID)
	if err != nil {
		// Не критично, если статистика упадет, главное прогресс сохранился. Но логируем.
		log.Printf("Error updating user stats: %v", err)
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"message": "Progress saved"})
}

// --- GetLevels ( ▼▼▼ ОБНОВЛЕННАЯ ЛОГИКА ▼▼▼ ) ---
func (h *ApiHandler) GetLevels(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}

	// 1. Уровни
	rows, err := h.DB.Query("SELECT id, title FROM levels ORDER BY title")
	if err != nil {
		http.Error(w, "Failed to query levels", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	levels := []models.Level{}
	for rows.Next() {
		var l models.Level
		rows.Scan(&l.ID, &l.Title)
		levels = append(levels, l)
	}

	// 2. Всего уроков
	var totalLessons int
	h.DB.QueryRow("SELECT COUNT(id) FROM lessons").Scan(&totalLessons)

	// 3. Пройдено уроков
	var completedLessons int
	completedQuery := `
		SELECT COUNT(DISTINCT l.id) FROM lessons l WHERE (
			(SELECT COUNT(id) FROM sentences s WHERE s.lesson_id = l.id) > 0 AND
			(SELECT COUNT(id) FROM sentences s WHERE s.lesson_id = l.id) = 
			(SELECT COUNT(DISTINCT s_up.id) FROM sentences s_up JOIN user_progress up ON s_up.id = up.sentence_id WHERE s_up.lesson_id = l.id AND up.user_id = $1 AND up.status = 'mastered')
		)
	`
	h.DB.QueryRow(completedQuery, userID).Scan(&completedLessons)

	// 4. Время обучения
	var totalStudyTime time.Duration
	var sessionTimeout = 15 * time.Minute
	timeRows, err := h.DB.Query("SELECT updated_at FROM user_progress WHERE user_id = $1 ORDER BY updated_at ASC", userID)
	if err == nil {
		var lastTime time.Time
		var firstTime = true
		for timeRows.Next() {
			var currentTime time.Time
			if err := timeRows.Scan(&currentTime); err != nil { continue }
			if firstTime { lastTime = currentTime; firstTime = false } else {
				duration := currentTime.Sub(lastTime)
				if duration < sessionTimeout { totalStudyTime += duration }
				lastTime = currentTime
			}
		}
		timeRows.Close()
	}

	// 5. ▼▼▼ НОВОЕ: ПОЛУЧАЕМ ТОЧНОСТЬ ▼▼▼
	var totalAttempts, totalCorrect int
	err = h.DB.QueryRow("SELECT total_attempts, total_correct FROM users WHERE id = $1", userID).Scan(&totalAttempts, &totalCorrect)
	if err != nil {
		log.Printf("Error getting accuracy: %v", err)
		totalAttempts = 0
		totalCorrect = 0
	}

	// Считаем процент
	var accuracy float64 = 0
	if totalAttempts > 0 {
		accuracy = (float64(totalCorrect) / float64(totalAttempts)) * 100
	}

	// 6. Ответ
	data := struct {
		Levels           []models.Level `json:"levels"`
		CompletedLessons int            `json:"completed_lessons"`
		TotalLessons     int            `json:"total_lessons"`
		StudyTimeHours   float64        `json:"study_time_hours"`
		Accuracy         float64        `json:"accuracy"` // <--- НОВОЕ ПОЛЕ
	}{
		Levels:           levels,
		CompletedLessons: completedLessons,
		TotalLessons:     totalLessons,
		StudyTimeHours:   totalStudyTime.Hours(),
		Accuracy:         accuracy, // <--- ОТПРАВЛЯЕМ
	}

	respondWithJSON(w, http.StatusOK, data)
}


// --- GetLessonsByLevel ---
func (h *ApiHandler) GetLessonsByLevel(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}
	vars := mux.Vars(r)
	levelID, err := strconv.Atoi(vars["level_id"])
	if err != nil { http.Error(w, "Invalid level ID", http.StatusBadRequest); return }

	sqlQuery := `
		SELECT l.id, l.level_id, l.lesson_number, l.title,
			COALESCE(COUNT(DISTINCT s.id), 0) AS total_sentences,
			COALESCE(COUNT(DISTINCT up.id), 0) AS completed_sentences
		FROM lessons l
		LEFT JOIN sentences s ON l.id = s.lesson_id
		LEFT JOIN user_progress up ON s.id = up.sentence_id AND up.user_id = $1 AND up.status = 'mastered'
		WHERE l.level_id = $2
		GROUP BY l.id, l.level_id, l.lesson_number, l.title
		ORDER BY l.lesson_number;
	`
	rows, err := h.DB.Query(sqlQuery, userID, levelID)
	if err != nil { http.Error(w, "Failed to query lessons", http.StatusInternalServerError); return }
	defer rows.Close()

	lessons := []models.Lesson{}
	for rows.Next() {
		var l models.Lesson
		if err := rows.Scan(&l.ID, &l.LevelID, &l.LessonNumber, &l.Title, &l.TotalSentences, &l.CompletedSentences); err != nil { continue }
		lessons = append(lessons, l)
	}
	respondWithJSON(w, http.StatusOK, lessons)
}

// --- GetSentencesByLesson ---
func (h *ApiHandler) GetSentencesByLesson(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok { respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)"); return }
	vars := mux.Vars(r)
	lessonID, err := strconv.Atoi(vars["lesson_id"])
	if err != nil { http.Error(w, "Invalid lesson ID", http.StatusBadRequest); return }

	sqlQuery := `SELECT s.id, s.lesson_id, s.order_number, s.prompt_ru, s.answer_en, s.transcription, s.audio_path, up.status, up.correct_streak FROM sentences s LEFT JOIN user_progress up ON s.id = up.sentence_id AND up.user_id = $1 WHERE s.lesson_id = $2 ORDER BY s.order_number;`
	rows, err := h.DB.Query(sqlQuery, userID, lessonID)
	if err != nil { http.Error(w, "Failed to query sentences", http.StatusInternalServerError); return }
	defer rows.Close()

	sentences := []models.Sentence{}
	for rows.Next() {
		var s models.Sentence
		if err := rows.Scan(&s.ID, &s.LessonID, &s.OrderNumber, &s.PromptRU, &s.AnswerEN, &s.Transcription, &s.AudioPath, &s.Status, &s.CorrectStreak); err != nil { continue }
		sentences = append(sentences, s)
	}
	respondWithJSON(w, http.StatusOK, sentences)
}


// --- ExplainError ---
func (h *ApiHandler) ExplainError(w http.ResponseWriter, r *http.Request) {
    var req ExplainErrorRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request"); return }
	if req.UserAnswerEN == "" { respondWithJSON(w, http.StatusOK, map[string]string{"explanation": "Пустой ответ."}); return }
    hfToken := os.Getenv("HUGGINGFACE_TOKEN")
    if hfToken == "" { respondWithError(w, http.StatusInternalServerError, "AI config error"); return }

    model := "meta-llama/Meta-Llama-3-8B-Instruct"
    apiURL := "https://router.huggingface.co/v1/chat/completions"

	prompt := fmt.Sprintf(`Ты — репетитор по английскому. Объясни КРАТКО ошибку (1-2 предл). Русский: "%s", Правильно: "%s". Не здоровайся.`, req.PromptRU, req.CorrectEN)

    payload := hfChatRequest{ Model: model, Messages: []hfMessage{ {Role: "user", Content: prompt}, }, Stream: false }
    payloadBytes, _ := json.Marshal(payload)
    hfReq, _ := http.NewRequestWithContext(r.Context(), "POST", apiURL, bytes.NewBuffer(payloadBytes))
    hfReq.Header.Set("Authorization", "Bearer "+hfToken)
    hfReq.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 30 * time.Second}
    hfResp, err := client.Do(hfReq)
    if err != nil { respondWithError(w, http.StatusInternalServerError, "AI connect error"); return }
    defer hfResp.Body.Close()
    
    var result hfChatResponse
    if err := json.NewDecoder(hfResp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
        respondWithError(w, http.StatusInternalServerError, "AI parse error")
        return
    }
    respondWithJSON(w, http.StatusOK, map[string]string{"explanation": result.Choices[0].Message.Content})
}

// --- Helpers ---
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}