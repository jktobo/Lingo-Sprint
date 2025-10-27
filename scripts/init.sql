-- Создаем таблицы только если они еще не существуют
CREATE TABLE IF NOT EXISTS levels (
    id SERIAL PRIMARY KEY,
    title VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS lessons (
    id SERIAL PRIMARY KEY,
    level_id INT NOT NULL REFERENCES levels(id),
    lesson_number INT NOT NULL,
    title VARCHAR(255)
);

CREATE TABLE IF NOT EXISTS sentences (
    id SERIAL PRIMARY KEY,
    lesson_id INT NOT NULL REFERENCES lessons(id),
    order_number INT NOT NULL, -- Порядковый номер в уроке (1, 2, 3...)
    prompt_ru TEXT NOT NULL,
    answer_en TEXT NOT NULL,
    transcription VARCHAR(255),
    audio_path VARCHAR(1024)
);

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    subscription_status VARCHAR(20) DEFAULT 'free' NOT NULL, -- 'free', 'premium'
    stripe_customer_id VARCHAR(255) UNIQUE
);

CREATE TABLE IF NOT EXISTS user_progress (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    sentence_id INT NOT NULL REFERENCES sentences(id),
    status VARCHAR(20) DEFAULT 'new' NOT NULL, -- 'new', 'learning', 'mastered'
    correct_streak INT DEFAULT 0 NOT NULL,
    next_review_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, sentence_id)
);

-- Индексы для ускорения запросов
CREATE INDEX IF NOT EXISTS idx_user_progress_review ON user_progress (user_id, next_review_date);
CREATE INDEX IF NOT EXISTS idx_sentences_lesson ON sentences (lesson_id);

-- === НОВОЕ: Добавляем все уровни ===
INSERT INTO levels (title) VALUES ('A0') ON CONFLICT (title) DO NOTHING;
INSERT INTO levels (title) VALUES ('A1') ON CONFLICT (title) DO NOTHING;
INSERT INTO levels (title) VALUES ('A2') ON CONFLICT (title) DO NOTHING;
INSERT INTO levels (title) VALUES ('B1') ON CONFLICT (title) DO NOTHING;
INSERT INTO levels (title) VALUES ('B2') ON CONFLICT (title) DO NOTHING;
INSERT INTO levels (title) VALUES ('C1') ON CONFLICT (title) DO NOTHING;