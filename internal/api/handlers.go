package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

type ApiHandler struct {
	DB *sql.DB
}

func NewApiHandler(db *sql.DB) *ApiHandler {
	return &ApiHandler{DB: db}
}

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Claims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

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

type SaveProgressRequest struct {
	SentenceID int  `json:"sentence_id"`
	IsCorrect  bool `json:"is_correct"`
}
type ExplainErrorRequest struct {
	PromptRU     string `json:"prompt_ru"`
	CorrectEN    string `json:"correct_en"`
	UserAnswerEN string `json:"user_answer_en"`
}
type hfMessage struct { Role string `json:"role"`; Content string `json:"content"` }
type hfChatRequest struct { Model string `json:"model"`; Messages []hfMessage `json:"messages"`; Stream bool `json:"stream"` }
type hfChatResponse struct { Choices []struct { Message struct { Role string `json:"role"`; Content string `json:"content"` } `json:"message"` } `json:"choices"`; Error *struct { Message string `json:"message"` } `json:"error,omitempty"` }


// --- SaveProgress (ИСПРАВЛЕНО: СЧЕТЧИК ОШИБОК) ---
func (h *ApiHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok { respondWithError(w, http.StatusUnauthorized, "Invalid token"); return }

	var req SaveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid payload"); return }

	var nextStatus string
	var nextStreak int
	var nextReview time.Time
	now := time.Now()
	
	// Статистика для users table
	incrementCorrect := 0
	
	// Счетчик ошибок для user_progress table (для звезд)
	incrementMistake := 0

	if req.IsCorrect {
		nextStatus = "mastered"
		nextStreak = 1
		nextReview = now.Add(100 * 365 * 24 * time.Hour)
		incrementCorrect = 1
		// Ошибок не добавляем, но старые (если были) останутся в базе благодаря UPDATE mistake_count = mistake_count + 0
	} else {
		nextStatus = "learning"
		nextStreak = 0
		nextReview = now
		incrementCorrect = 0
		incrementMistake = 1 // Увеличиваем счетчик ошибок
	}

	// Теперь мы обновляем mistake_count. 
	// Если была ошибка -> mistake_count = mistake_count + 1
	// Если верно -> mistake_count = mistake_count + 0 (сохраняем историю ошибок)
	sqlProgress := `
		INSERT INTO user_progress (user_id, sentence_id, status, correct_streak, next_review_date, updated_at, mistake_count) 
		VALUES ($1, $2, $3, $4, $5, NOW(), $6) 
		ON CONFLICT (user_id, sentence_id) 
		DO UPDATE SET 
			status = EXCLUDED.status, 
			correct_streak = EXCLUDED.correct_streak, 
			next_review_date = EXCLUDED.next_review_date,
			updated_at = NOW(),
			mistake_count = user_progress.mistake_count + EXCLUDED.mistake_count;
	`
	_, err := h.DB.Exec(sqlProgress, userID, req.SentenceID, nextStatus, nextStreak, nextReview, incrementMistake)
	if err != nil {
		log.Printf("Error saving sentence progress: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save sentence progress")
		return
	}

	sqlStats := `UPDATE users SET total_attempts = total_attempts + 1, total_correct = total_correct + $1 WHERE id = $2`
	_, err = h.DB.Exec(sqlStats, incrementCorrect, userID)
	if err != nil { log.Printf("Error updating user stats: %v", err) }

	respondWithJSON(w, http.StatusOK, map[string]string{"message": "Progress saved"})
}

// --- GetLevels ---
func (h *ApiHandler) GetLevels(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok { respondWithError(w, http.StatusUnauthorized, "Invalid token"); return }

	rows, err := h.DB.Query("SELECT id, title FROM levels ORDER BY title")
	if err != nil { http.Error(w, "Failed to query levels", http.StatusInternalServerError); return }
	defer rows.Close()
	levels := []models.Level{}
	for rows.Next() {
		var l models.Level
		rows.Scan(&l.ID, &l.Title)
		levels = append(levels, l)
	}

	var totalLessons int
	h.DB.QueryRow("SELECT COUNT(id) FROM lessons").Scan(&totalLessons)

	var completedLessons int
	completedQuery := `
		SELECT COUNT(DISTINCT l.id) FROM lessons l WHERE (
			(SELECT COUNT(id) FROM sentences s WHERE s.lesson_id = l.id) > 0 AND
			(SELECT COUNT(id) FROM sentences s WHERE s.lesson_id = l.id) = 
			(SELECT COUNT(DISTINCT s_up.id) FROM sentences s_up JOIN user_progress up ON s_up.id = up.sentence_id WHERE s_up.lesson_id = l.id AND up.user_id = $1 AND up.status = 'mastered')
		)
	`
	h.DB.QueryRow(completedQuery, userID).Scan(&completedLessons)

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

	var totalAttempts, totalCorrect int
	err = h.DB.QueryRow("SELECT total_attempts, total_correct FROM users WHERE id = $1", userID).Scan(&totalAttempts, &totalCorrect)
	if err != nil { totalAttempts = 0; totalCorrect = 0 }
	var accuracy float64 = 0
	if totalAttempts > 0 { accuracy = (float64(totalCorrect) / float64(totalAttempts)) * 100 }

	// ▼▼▼ ИСПРАВЛЕННЫЙ ПОДСЧЕТ ЗВЕЗД ▼▼▼
	// Считаем предложения, где mistake_count > 0
	starsQuery := `
		SELECT
			l.id,
			(SELECT COUNT(*) FROM sentences WHERE lesson_id = l.id) AS total,
			(SELECT COUNT(*) FROM sentences s JOIN user_progress up ON s.id = up.sentence_id WHERE s.lesson_id = l.id AND up.user_id = $1 AND up.status = 'mastered') AS completed,
			(SELECT COUNT(DISTINCT sentence_id) FROM user_progress up JOIN sentences s ON up.sentence_id = s.id WHERE s.lesson_id = l.id AND up.user_id = $1 AND up.mistake_count > 0) AS errors
		FROM lessons l
	`
	starRows, err := h.DB.Query(starsQuery, userID)
	var earnedStarsTotal int = 0

	if err == nil {
		for starRows.Next() {
			var id, total, completed, errs int
			starRows.Scan(&id, &total, &completed, &errs)
			if total > 0 && completed == total {
				if errs == 0 {
					earnedStarsTotal += 3
				} else {
					errPct := float64(errs) / float64(total)
					if errPct < 0.05 {
						earnedStarsTotal += 2
					} else {
						earnedStarsTotal += 1
					}
				}
			}
		}
		starRows.Close()
	}
	// ▲▲▲ КОНЕЦ ИСПРАВЛЕНИЙ ▲▲▲

	data := struct {
		Levels           []models.Level `json:"levels"`
		CompletedLessons int            `json:"completed_lessons"`
		TotalLessons     int            `json:"total_lessons"`
		StudyTimeHours   float64        `json:"study_time_hours"`
		Accuracy         float64        `json:"accuracy"`
		EarnedStars      int            `json:"earned_stars"` 
		TotalStars       int            `json:"total_stars"` 
	}{
		Levels:           levels,
		CompletedLessons: completedLessons,
		TotalLessons:     totalLessons,
		StudyTimeHours:   totalStudyTime.Hours(),
		Accuracy:         accuracy,
		EarnedStars:      earnedStarsTotal,
		TotalStars:       totalLessons * 3,
	}
	respondWithJSON(w, http.StatusOK, data)
}


// --- GetLessonsByLevel ( ▼▼▼ ИСПРАВЛЕННАЯ ЛОГИКА ЗВЕЗД ▼▼▼ ) ---
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
		SELECT
			l.id, l.level_id, l.lesson_number, l.title,
			(SELECT COUNT(*) FROM sentences WHERE lesson_id = l.id) AS total_sentences,
			(SELECT COUNT(*) FROM sentences s JOIN user_progress up ON s.id = up.sentence_id WHERE s.lesson_id = l.id AND up.user_id = $1 AND up.status = 'mastered') AS completed_sentences,
			
			-- Ищем предложения, где пользователь допустил ошибку (mistake_count > 0)
			(SELECT COUNT(DISTINCT sentence_id) 
             FROM user_progress up 
             JOIN sentences s ON up.sentence_id = s.id 
             WHERE s.lesson_id = l.id AND up.user_id = $1 AND up.mistake_count > 0
            ) AS sentences_with_errors
            
		FROM lessons l
		WHERE l.level_id = $2
		ORDER BY l.lesson_number;
	`
	
	rows, err := h.DB.Query(sqlQuery, userID, levelID)
	if err != nil { 
		log.Printf("DB ERROR: Failed to query lessons. SQL Error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return 
	}
	defer rows.Close()

	lessons := []models.Lesson{}
	for rows.Next() {
		var l models.Lesson
        var sentencesWithErrors int 
		
		if err := rows.Scan(&l.ID, &l.LevelID, &l.LessonNumber, &l.Title, &l.TotalSentences, &l.CompletedSentences, &sentencesWithErrors); err != nil { 
			log.Printf("DB SCAN ERROR: skipping row: %v", err)
			continue
		}
        
        l.SentencesWithErrors = sentencesWithErrors 
        isLessonCompleted := (l.TotalSentences > 0 && l.CompletedSentences == l.TotalSentences)
        
        if isLessonCompleted {
            if l.SentencesWithErrors == 0 {
                l.SentencesWithErrors = 3 
            } else {
                errorPercentage := float64(l.SentencesWithErrors) / float64(l.TotalSentences)
                if errorPercentage < 0.05 {
                    l.SentencesWithErrors = 2 
                } else {
                    l.SentencesWithErrors = 1 
                }
            }
        } else {
            l.SentencesWithErrors = 0
        }
        
		lessons = append(lessons, l)
	}
	respondWithJSON(w, http.StatusOK, lessons)
}

// --- GetSentencesByLesson ---
func (h *ApiHandler) GetSentencesByLesson(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok { respondWithError(w, http.StatusUnauthorized, "Invalid token"); return }
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

	prompt := fmt.Sprintf(`Ты — репетитор по английскому. Студент не смог правильно перевести предложение.

- Русский: "%s"
- Правильный ответ: "%s"

Твоя задача — ОЧЕНЬ КРАТКО (1-2 предложения) на РУССКОМ языке объяснить, на какое **основное грамматическое правило** нужно обратить внимание в "Правильном ответе".

Начинай:`,
    req.PromptRU, req.CorrectEN)

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