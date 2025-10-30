package api

import (
	"bytes"
	// "context" // <-- ИМПОРТ ЕСТЬ
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
	// Используем JwtKey из config.go
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


// --- SaveProgress (Логика "Один раз и готово") ---
func (h *ApiHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int) // <-- ИСПОЛЬЗУЕТ context
	if !ok {
		log.Println("!!! SaveProgress ERROR: Could not get user ID from context")
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}
	log.Printf("SaveProgress: UserID=%d", userID)

	var req SaveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("!!! SaveProgress ERROR: Could not decode JSON for UserID=%d: %v", userID, err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	log.Printf("SaveProgress: Received SentenceID=%d, IsCorrect=%t", req.SentenceID, req.IsCorrect)

	var nextStatus string
	var nextStreak int
	var nextReview time.Time
	now := time.Now()

	if req.IsCorrect {
		nextStatus = "mastered"
		nextStreak = 1
		nextReview = now.Add(100 * 365 * 24 * time.Hour)
		log.Printf("SaveProgress Logic: CORRECT. Setting status to mastered.")
	} else {
		nextStatus = "learning"
		nextStreak = 0
		nextReview = now
		log.Printf("SaveProgress Logic: INCORRECT. Setting status to learning, resetting streak.")
	}

	sqlStatement := `INSERT INTO user_progress (user_id, sentence_id, status, correct_streak, next_review_date) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (user_id, sentence_id) DO UPDATE SET status = EXCLUDED.status, correct_streak = EXCLUDED.correct_streak, next_review_date = EXCLUDED.next_review_date;`
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

// --- GetLevels ---
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

// --- GetLessonsByLevel ---
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

// --- GetSentencesByLesson ---
func (h *ApiHandler) GetSentencesByLesson(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int) // <-- ИСПОЛЬЗУЕТ context
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}
	vars := mux.Vars(r)
	lessonID, err := strconv.Atoi(vars["lesson_id"])
	if err != nil {
		http.Error(w, "Invalid lesson ID", http.StatusBadRequest)
		return
	}

	sqlQuery := `SELECT s.id, s.lesson_id, s.order_number, s.prompt_ru, s.answer_en, s.transcription, s.audio_path, up.status, up.correct_streak FROM sentences s LEFT JOIN user_progress up ON s.id = up.sentence_id AND up.user_id = $1 WHERE s.lesson_id = $2 ORDER BY s.order_number;`
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
		if err := rows.Scan(&s.ID, &s.LessonID, &s.OrderNumber, &s.PromptRU, &s.AnswerEN, &s.Transcription, &s.AudioPath, &s.Status, &s.CorrectStreak); err != nil {
			log.Printf("Error scanning sentence with progress: %v", err)
			continue
		}
		sentences = append(sentences, s)
	}
	respondWithJSON(w, http.StatusOK, sentences)
}


// --- ИСПРАВЛЕННАЯ: ExplainError ---
func (h *ApiHandler) ExplainError(w http.ResponseWriter, r *http.Request) {
    var req ExplainErrorRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondWithError(w, http.StatusBadRequest, "Invalid request payload")
        return
    }
    hfToken := os.Getenv("HUGGINGFACE_TOKEN")
    if hfToken == "" {
        log.Println("HUGGINGFACE_TOKEN environment variable is not set!")
        respondWithError(w, http.StatusInternalServerError, "AI service configuration error")
        return
    }

    model := "meta-llama/Meta-Llama-3-8B-Instruct"
    apiURL := "https://router.huggingface.co/v1/chat/completions"

    prompt := fmt.Sprintf(`You are a helpful English tutor explaining a mistake to a student learning English.
The student was asked to translate the Russian sentence: "%s"
The correct English translation is: "%s"
The student answered: "%s"
Explain the student's mistake clearly and concisely in Russian. Focus only on the main error. Be encouraging.`,
        req.PromptRU, req.CorrectEN, req.UserAnswerEN)

    payload := hfChatRequest{
        Model: model,
        Messages: []hfMessage{ {Role: "user", Content: prompt}, },
        Stream: false,
    }
    payloadBytes, err := json.Marshal(payload)
    if err != nil { 
		log.Printf("Failed to marshal HF payload: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI request preparation failed")
		return 
	}

    // ИСПОЛЬЗУЕМ r.Context()
    hfReq, err := http.NewRequestWithContext(r.Context(), "POST", apiURL, bytes.NewBuffer(payloadBytes)) // <-- ИСПОЛЬЗУЕТ context
    if err != nil { 
		log.Printf("Failed to create HF request: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI request creation failed")
		return 
	}

    hfReq.Header.Set("Authorization", "Bearer "+hfToken)
    hfReq.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 30 * time.Second}
    log.Printf("Sending request to NEW CHAT endpoint: %s", apiURL)
    hfResp, err := client.Do(hfReq)
    if err != nil { 
		log.Printf("Direct HF API call failed (network/timeout): %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI service connection error")
		return 
	}
    defer hfResp.Body.Close()

    bodyBytes, err := io.ReadAll(hfResp.Body)
    if err != nil { 
		log.Printf("Failed to read HF response body: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI response reading failed")
		return 
	}
    bodyString := string(bodyBytes)
    log.Printf("HF Chat Response Status: %s", hfResp.Status)
    log.Printf("HF Chat Response Body: %s", bodyString)

    if hfResp.StatusCode != http.StatusOK {
        log.Printf("Hugging Face returned non-OK status: %d", hfResp.StatusCode);
        respondWithError(w, hfResp.StatusCode, fmt.Sprintf("AI service error (%d): %s", hfResp.StatusCode, bodyString));
        return
    }

    var result hfChatResponse
    err = json.Unmarshal(bodyBytes, &result); if err != nil { 
		log.Printf("Could not parse HF Chat response JSON: %v", err)
		respondWithError(w, http.StatusInternalServerError, "AI response parsing failed")
		return 
	}
    if result.Error != nil && result.Error.Message != "" { 
		log.Printf("HF API returned an error message in body: %s", result.Error.Message)
		respondWithError(w, http.StatusInternalServerError, result.Error.Message)
		return 
	}
    if len(result.Choices) == 0 { 
		log.Printf("HF API returned 0 choices.")
		respondWithError(w, http.StatusInternalServerError, "AI returned no response")
		return 
	}
    completion := result.Choices[0].Message.Content
    
    respondWithJSON(w, http.StatusOK, map[string]string{"explanation": completion}) // <-- ИСПРАВЛЕНО
}

// --- Вспомогательные функции ---
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}