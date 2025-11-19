package models

import "database/sql"

// Level представляет один уровень (A0, A1...)
type Level struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// Lesson представляет один урок
type Lesson struct {
	ID           int    `json:"id"`
	LevelID      int    `json:"level_id"`
	LessonNumber int    `json:"lesson_number"`
	Title        string `json:"title"`

	// Данные для прогресса
	TotalSentences     int `json:"total_sentences"`
	CompletedSentences int `json:"completed_sentences"`
    
    // ▼▼▼ НОВОЕ ПОЛЕ ▼▼▼
	SentencesWithErrors int `json:"sentences_with_errors"` 
    // ▲▲▲ КОНЕЦ НОВОГО ПОЛЯ ▲▲▲
}

// Sentence представляет одно предложение С УЧЕТОМ ПРОГРЕССА
type Sentence struct {
	ID          int    `json:"id"`
	LessonID    int    `json:"lesson_id"`
	OrderNumber int    `json:"order_number"`
	PromptRU    string `json:"prompt_ru"`
	AnswerEN    string `json:"answer_en"`

	Transcription sql.NullString `json:"transcription"`
	AudioPath     sql.NullString `json:"audio_path"`

	Status        sql.NullString `json:"status"`
	CorrectStreak sql.NullInt32  `json:"correct_streak"`
}