package api

import (
	"database/sql"
	"encoding/json"
	"errors" // Добавили
	"log"
	"net/http"
	"strconv"
	"time" // Добавили

	"lingo-sprint/internal/models"

	"github.com/golang-jwt/jwt/v5" // Добавили
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt" // Добавили
)

// ВАЖНО: Это ваш "секретный ключ" для подписи токенов.
// В реальном проекте он должен быть в .env файле, а не в коде!
var jwtKey = []byte("my_very_secret_and_long_key_32_bytes")

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
	tokenString, err := token.SignedString(jwtKey)
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

// --- ОБНОВЛЕННАЯ ФУНКЦИЯ: SaveProgress (с новой логикой) ---
func (h *ApiHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	// 1. Получаем ID пользователя
	userID, ok := r.Context().Value(ContextUserIDKey).(int)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token (no user ID)")
		return
	}

	// 2. Читаем JSON
	var req SaveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// --- НОВАЯ ЛОГИКА: Сначала проверяем текущий прогресс ---
	var currentStatus sql.NullString
	var currentStreak sql.NullInt32
	err := h.DB.QueryRow(
		"SELECT status, correct_streak FROM user_progress WHERE user_id = $1 AND sentence_id = $2",
		userID, req.SentenceID,
	).Scan(&currentStatus, &currentStreak)

	// Определяем, видим ли мы это предложение впервые
	isFirstTime := errors.Is(err, sql.ErrNoRows) || !currentStatus.Valid || currentStatus.String == "new"

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("Error fetching current progress: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch progress")
		return
	}
	// --------------------------------------------------------

	// --- Определяем НОВЫЙ статус и счетчик ---
	var nextStatus string
	var nextStreak int
	var nextReview time.Time // (Пока оставляем простую логику +1 день/-100 лет)

	if req.IsCorrect {
		if isFirstTime {
			// ПЕРВЫЙ РАЗ и ПРАВИЛЬНО -> Mastered
			nextStatus = "mastered"
			nextStreak = 1 // Начинаем счетчик с 1
			nextReview = time.Now().Add(100 * 365 * 24 * time.Hour) // Убираем надолго
		} else if currentStatus.String == "learning" {
			// НЕ ПЕРВЫЙ РАЗ, ПРАВИЛЬНО (в процессе обучения)
			nextStreak = int(currentStreak.Int32) + 1
			if nextStreak >= 3 {
				// Достигли 3 правильных ответов подряд -> Mastered
				nextStatus = "mastered"
				nextReview = time.Now().Add(100 * 365 * 24 * time.Hour)
			} else {
				// Еще не 3 раза, продолжаем учить -> Learning
				nextStatus = "learning"
				nextReview = time.Now().Add(24 * time.Hour) // Повторим завтра
			}
		} else {
			// Уже было 'mastered', просто обновляем время (маловероятно, но на всякий случай)
			nextStatus = "mastered"
			nextStreak = 3 // Оставляем 3
			nextReview = time.Now().Add(100 * 365 * 24 * time.Hour)
		}
	} else { // НЕПРАВИЛЬНО
		// Неважно, первый раз или нет -> Learning, сброс счетчика
		nextStatus = "learning"
		nextStreak = 0 // Сбрасываем счетчик
		nextReview = time.Now() // Повторить немедленно (или в следующей сессии)
	}
	// ---------------------------------------------

	// --- Обновляем или вставляем (UPSERT) ---
	sqlStatement := `
		INSERT INTO user_progress (user_id, sentence_id, status, correct_streak, next_review_date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, sentence_id)
		DO UPDATE SET
			status = EXCLUDED.status,
			correct_streak = EXCLUDED.correct_streak,
			next_review_date = EXCLUDED.next_review_date;
	`

	_, err = h.DB.Exec(sqlStatement, userID, req.SentenceID, nextStatus, nextStreak, nextReview)
	if err != nil {
		log.Printf("Failed to save progress for user %d, sentence %d: %v", userID, req.SentenceID, err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save progress")
		return
	}

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