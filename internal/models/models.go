package models

import "database/sql" // <-- ДОБАВЛЕНО

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
}

// Sentence представляет одно предложение С УЧЕТОМ ПРОГРЕССА
type Sentence struct {
	ID            int    `json:"id"`
	LessonID      int    `json:"lesson_id"`
	OrderNumber   int    `json:"order_number"`
	PromptRU      string `json:"prompt_ru"`
	AnswerEN      string `json:"answer_en"`
	Transcription string `json:"transcription"`
	AudioPath     string `json:"audio_path"`

	// === НОВЫЕ ПОЛЯ (могут быть NULL) ===
	// Мы используем sql.NullString/Int32, чтобы Go
	// мог обработать NULL из LEFT JOIN.
	// При конвертации в JSON они станут "null" или "значением".
	Status        sql.NullString `json:"status"`
	CorrectStreak sql.NullInt32  `json:"correct_streak"`
}

// (Мы добавим User и UserProgress позже, когда будем делать логин)