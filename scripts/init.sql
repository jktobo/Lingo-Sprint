-- Таблица уровней (A0, A1, B1...)
CREATE TABLE levels (
    id SERIAL PRIMARY KEY,
    title VARCHAR(50) NOT NULL UNIQUE
);

-- Таблица уроков (Урок 1, Урок 2...)
CREATE TABLE lessons (
    id SERIAL PRIMARY KEY,
    level_id INT NOT NULL REFERENCES levels(id),
    lesson_number INT NOT NULL,
    title VARCHAR(255)
);

-- Главная таблица с предложениями
CREATE TABLE sentences (
    id SERIAL PRIMARY KEY,
    lesson_id INT NOT NULL REFERENCES lessons(id),
    order_number INT NOT NULL, -- Порядковый номер в уроке (1, 2, 3...)
    prompt_ru TEXT NOT NULL,
    answer_en TEXT NOT NULL,
    transcription VARCHAR(255),
    audio_path VARCHAR(1024) NOT NULL -- Путь к файлу в S3/Yandex Storage
);

-- Таблица пользователей
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Поля для монетизации
    subscription_status VARCHAR(20) DEFAULT 'free' NOT NULL, -- 'free', 'premium'
    stripe_customer_id VARCHAR(255) UNIQUE -- ID из Stripe/Optima/KICB
);

-- Таблица для отслеживания прогресса (для интервального повторения)
CREATE TABLE user_progress (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    sentence_id INT NOT NULL REFERENCES sentences(id),
    status VARCHAR(20) DEFAULT 'new' NOT NULL, -- 'new', 'learning', 'mastered'
    correct_streak INT DEFAULT 0 NOT NULL,
    next_review_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(user_id, sentence_id) -- Один юзер - одно предложение
);

-- Индексы для ускорения запросов
CREATE INDEX idx_user_progress_review ON user_progress (user_id, next_review_date);
CREATE INDEX idx_sentences_lesson ON sentences (lesson_id);