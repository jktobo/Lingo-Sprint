document.addEventListener("DOMContentLoaded", () => {
    // === Элементы DOM ===
    const views = {
        auth: document.getElementById("auth-view"),
        dashboard: document.getElementById("dashboard-view"),
        trainer: document.getElementById("trainer-view"),
    };
    const loginForm = document.getElementById("login-form");
    const registerForm = document.getElementById("register-form");
    const showRegisterBtn = document.getElementById("show-register");
    const showLoginBtn = document.getElementById("show-login");
    const loginError = document.getElementById("login-error");
    const registerError = document.getElementById("register-error");
    const logoutButton = document.getElementById("logout-button");
    const headerStartButton = document.getElementById("header-start-button");
    const heroStartButton = document.getElementById("hero-start-button");
    const authOverlay = document.getElementById("auth-overlay");
    const authModal = document.getElementById("auth-modal");
    const closeModalButton = document.getElementById("close-modal-button");
    const levelsContainer = document.getElementById("levels-container");
    const lessonsContainer = document.getElementById("lessons-container");
    const lessonTitle = document.getElementById("lesson-title");
    const promptRu = document.getElementById("prompt-ru");
    const userAnswer = document.getElementById("user-answer");
    const checkAnswerBtn = document.getElementById("check-answer");
    const nextSentenceBtn = document.getElementById("next-sentence");
    const backToLessonsBtn = document.getElementById("back-to-lessons");
    const playAudioBtn = document.getElementById("play-audio");
    const feedback = document.getElementById("feedback");
    const feedbackText = document.getElementById("feedback-text");
    const correctAnswer = document.getElementById("correct-answer");
    const aiExplanation = document.getElementById("ai-explanation");
    const progressBar = document.getElementById("progress-bar-inner");
    const progressText = document.getElementById("progress-text");

    // === Состояние приложения ===
    let state = {
        token: localStorage.getItem("token") || null,
        currentLessonId: null,
        levels: [], lessons: [], sentences: [],
        currentSentenceIndex: 0,
        currentAudio: null,
    };

    // === API-функции ===
    async function fetchProtected(url, options = {}) {
        if (!state.token) { console.error("Нет токена. Выход."); handleLogout(); return undefined; }
        const defaultHeaders = { 'Authorization': `Bearer ${state.token}`, 'Content-Type': 'application/json' };
        const config = { ...options, headers: { ...defaultHeaders, ...options.headers } };
        try { const response = await fetch(url, config); if (response.status === 401) { console.error("Токен невалиден (401). Выход."); handleLogout(); return undefined; } if (!response.ok) { console.error(`Ошибка API ${response.status}: ${response.statusText} для URL ${url}`); throw new Error(`API Error: ${response.status}`); } return response; } catch (error) { console.error("Ошибка сети fetchProtected:", error); throw error; }
    }
    async function fetchLevels() {
        try { const response = await fetchProtected("/api/levels"); if (!response) return; state.levels = await response.json(); renderLevels(); } catch (error) { console.error("Не удалось загрузить уровни:", error); }
    }
    async function fetchLessons(levelId) {
        if (isNaN(parseInt(levelId))) { console.error("Неверный ID уровня:", levelId); return; }
        try { const response = await fetchProtected(`/api/levels/${levelId}/lessons`); if (!response) return; state.lessons = await response.json(); renderLessons(); } catch (error) { console.error("Не удалось загрузить уроки:", error); }
    }
    async function fetchSentences(lessonId) {
        if (isNaN(parseInt(lessonId))) { console.error("Неверный ID урока:", lessonId); return; }
        state.currentLessonId = lessonId;
        try {
            const response = await fetchProtected(`/api/lessons/${lessonId}/sentences`);
            if (!response) return; state.sentences = await response.json();
             console.log("Sentences received:", JSON.stringify(state.sentences.slice(0, 3), null, 2));
            const firstUnansweredIndex = state.sentences.findIndex(s => !(s.status?.Valid && s.status.String === 'mastered'));
             console.log("Calculated firstUnansweredIndex:", firstUnansweredIndex);
            if (firstUnansweredIndex === -1 && state.sentences.length > 0) {
                 alert("Вы уже прошли этот урок! Показываем заново."); state.currentSentenceIndex = 0;
            } else { state.currentSentenceIndex = (firstUnansweredIndex === -1) ? 0 : firstUnansweredIndex; }
             console.log("Setting currentSentenceIndex to:", state.currentSentenceIndex);
            if (state.sentences.length > 0) { showView('trainer'); loadSentence(); }
            else { alert("В этом уроке нет предложений."); showView('dashboard'); }
        } catch (error) { console.error("Не удалось загрузить предложения:", error); }
    }
    async function saveProgress(sentenceId, isCorrect) {
        try { await fetchProtected("/api/progress/save", { method: "POST", body: JSON.stringify({ sentence_id: sentenceId, is_correct: isCorrect }) }); console.log(`Прогресс сохранен для предложения ${sentenceId}: ${isCorrect}`); } catch (error) { console.error("Не удалось сохранить прогресс:", error); }
    }


    async function fetchLessonSentencesData(lessonId) {
        if (isNaN(parseInt(lessonId))) {
            console.error("fetchLessonSentencesData: Неверный ID урока:", lessonId);
            return null;
        }
        try {
            const response = await fetchProtected(`/api/lessons/${lessonId}/sentences`);
            if (!response) return null; // fetchProtected уже обработал ошибку
            return await response.json();
        } catch (error) {
            console.error("Не удалось (re-fetch) загрузить предложения:", error);
            return null;
        }
    }

    
    /** (Раскомментировано) Запрашивает объяснение ошибки у AI */
    async function fetchErrorExplanation(promptRu, correctEn, userAnswerEn) {
        try {
            if (aiExplanation) {
                 aiExplanation.textContent = "🤖 Думаю над объяснением...";
                 aiExplanation.style.display = "block";
            }
            const response = await fetchProtected("/api/ai/explain-error", {
                method: "POST",
                body: JSON.stringify({
                    prompt_ru: promptRu, correct_en: correctEn, user_answer_en: userAnswerEn
                })
            });
            if (!response) return; // fetchProtected обработал ошибку (напр. 401)
            
            const data = await response.json();
            
            // Новая проверка: если ответ НЕ 200 OK, data будет содержать { "error": "..." }
            if (data.error) {
                console.error("Ошибка от AI API:", data.error);
                if (aiExplanation) {
                    aiExplanation.textContent = `Ошибка AI: ${data.error}`; // Показываем ошибку пользователю
                }
                return;
            }

             if (aiExplanation) {
                 aiExplanation.textContent = data.explanation || "Не удалось получить объяснение.";
             }
        } catch (error) {
            console.error("Не удалось получить объяснение AI:", error);
            if (aiExplanation) {
                 aiExplanation.textContent = "Не удалось связаться с AI-помощником.";
            }
        }
    }

    // === Функции Аутентификации ===
    async function handleLogin(e) {
        e.preventDefault(); if (loginError) loginError.style.display = "none"; const emailInput = document.getElementById("login-email"); const passwordInput = document.getElementById("login-password"); if (!emailInput || !passwordInput) { console.error("Login form elements missing"); return; } const email = emailInput.value; const password = passwordInput.value; try { const response = await fetch("/api/login", { method: "POST", headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) }); const data = await response.json(); if (!response.ok) throw new Error(data.error || "Ошибка входа"); saveToken(data.token); document.cookie = "auth_status=logged_in; path=/; max-age=" + 60*60*24*3; window.location.href = "/app"; } catch (error) { if (loginError) { loginError.textContent = error.message; loginError.style.display = "block"; } else { console.error("Login error element missing"); } }
    }
    async function handleRegister(e) {
        e.preventDefault(); if (registerError) registerError.style.display = "none"; const emailInput = document.getElementById("register-email"); const passwordInput = document.getElementById("register-password"); if (!emailInput || !passwordInput) { console.error("Register form elements missing"); return; } const email = emailInput.value; const password = passwordInput.value; if (password.length < 6) { if(registerError) { registerError.textContent = "Пароль не менее 6 симв."; registerError.style.display = "block"; } return; } try { const response = await fetch("/api/register", { method: "POST", headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) }); const data = await response.json(); if (!response.ok) throw new Error(data.error || "Ошибка регистрации"); alert("Регистрация успешна! Теперь войдите."); showAuthView('login'); if (registerForm) registerForm.reset(); } catch (error) { if (registerError) { registerError.textContent = error.message; registerError.style.display = "block"; } else { console.error("Register error element missing"); } }
    }
    function handleLogout() {
        localStorage.removeItem("token"); state.token = null; document.cookie = "auth_status=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
        state = { ...state, token: null, currentLessonId: null, levels: [], lessons: [], sentences: [], currentSentenceIndex: 0 };
        window.location.href = "/";
    }
    function saveToken(token) {
        localStorage.setItem("token", token); state.token = token;
    }

    // === Функции Рендеринга ===
    function renderLevels() {
        if (!levelsContainer) return; levelsContainer.innerHTML = "";
        state.levels.forEach(level => { const el = document.createElement("div"); el.className = "grid-item level-item"; el.textContent = level.title; el.dataset.id = level.id; levelsContainer.appendChild(el); });
    }
    function renderLessons() {
        if (!lessonsContainer) return; lessonsContainer.innerHTML = ""; if (!state.lessons || state.lessons.length === 0) { lessonsContainer.innerHTML = "<p style='color: var(--text-secondary);'>Выберите уровень.</p>"; return; }
        state.lessons.forEach(lesson => { const el = document.createElement("div"); el.className = "grid-item lesson-item"; el.textContent = `Урок ${lesson.lesson_number}`; el.dataset.id = lesson.id; el.dataset.title = lesson.title; lessonsContainer.appendChild(el); });
    }
    function loadSentence() {
        if (!promptRu || !userAnswer || !checkAnswerBtn || !feedback || !nextSentenceBtn || !progressBar || !progressText || !lessonTitle || !aiExplanation) {
             console.error("DOM элементы тренажера не найдены."); showView('dashboard'); return;
        }
        if (!state.sentences || state.currentSentenceIndex >= state.sentences.length) {
             console.log("Нет предложений или индекс вне диапазона."); if(state.sentences?.length > 0) alert("Урок пройден!"); showView('dashboard'); return;
        }
        const sentence = state.sentences[state.currentSentenceIndex];
        if (!sentence) { console.error("Объект sentence undef.", state.currentSentenceIndex); showView('dashboard'); return; }

        const lessonButton = lessonsContainer ? lessonsContainer.querySelector(`.lesson-item[data-id='${sentence.lesson_id}']`) : null;
        lessonTitle.textContent = lessonButton ? lessonButton.dataset.title : "Загрузка...";
        promptRu.textContent = sentence.prompt_ru || "[Нет текста]";
        state.currentAudio = sentence.audio_path;
        userAnswer.value = ""; userAnswer.disabled = false;
        checkAnswerBtn.style.display = "block";
        feedback.style.display = "none"; nextSentenceBtn.style.display = "none";
        if(aiExplanation) aiExplanation.style.display = "none";

        const totalNum = state.sentences.length;
        const currentNum = state.currentSentenceIndex + 1;
        const barProgress = (currentNum / totalNum) * 100;
         console.log("Обновление прогресс-бара:", progressBar, "Ширина:", `${barProgress}%`);
         console.log("Обновление текста прогресса:", progressText, "Текст:", `${currentNum} / ${totalNum}`);
        if (progressBar) progressBar.style.width = `${barProgress}%`;
        if (progressText) progressText.textContent = `${currentNum} / ${totalNum}`;

        if (userAnswer) userAnswer.focus();
    }

    // === Функции-Обработчики ===
    function handleCheckAnswer() {
        if (!userAnswer || !feedback || !checkAnswerBtn || !nextSentenceBtn || !correctAnswer || !feedbackText) return;
        if (!state.sentences || state.currentSentenceIndex >= state.sentences.length) return;
        const user = userAnswer.value.trim();
        const currentSentence = state.sentences[state.currentSentenceIndex];
        if (!currentSentence || typeof currentSentence.answer_en === 'undefined') return;
        const correct = currentSentence.answer_en.trim();
        const isCorrect = user.toLowerCase() === correct.toLowerCase();
        feedback.style.display = "block";
        feedbackText.textContent = isCorrect ? "Правильно! 👍" : "Ошибка 😞";
        correctAnswer.textContent = correct;
        feedback.className = isCorrect ? "correct" : "incorrect";
        
        // --- РАСКОММЕНТИРОВАНО ---
        if (!isCorrect && aiExplanation) {
            fetchErrorExplanation(currentSentence.prompt_ru, correct, user); // Вызываем AI
        } else if (aiExplanation) {
            aiExplanation.style.display = "none"; // Скрыть, если ответ правильный
        }
        // ------------------------
        
        saveProgress(currentSentence.id, isCorrect);
        nextSentenceBtn.dataset.wasCorrect = isCorrect.toString();
        userAnswer.disabled = true;
        checkAnswerBtn.style.display = "none";
        nextSentenceBtn.style.display = "block";
        nextSentenceBtn.focus();
    }
    
    function handleNextSentence() {
        if (!nextSentenceBtn || !state.sentences || state.sentences.length === 0) {
             console.error("handleNextSentence called with invalid state"); return;
        }
        console.log("--- handleNextSentence Start ---");
        console.log("Before increment, currentSentenceIndex:", state.currentSentenceIndex);
   
        state.currentSentenceIndex++;
        console.log("After increment, currentSentenceIndex:", state.currentSentenceIndex);
   
        while (
            state.currentSentenceIndex < state.sentences.length &&
            state.sentences[state.currentSentenceIndex]?.status?.Valid &&
            state.sentences[state.currentSentenceIndex].status.String === 'mastered'
        ) {
             console.log(`Пропуск (вперед) выученного предложения по индексу ${state.currentSentenceIndex}`);
             state.currentSentenceIndex++;
             console.log("After skipping, currentSentenceIndex:", state.currentSentenceIndex);
        }
   
        if (state.currentSentenceIndex >= state.sentences.length) {
             console.log("Дошли до конца урока (index >= length). Вызов checkAndCompleteLesson...");
             checkAndCompleteLesson();
        } else {
             console.log("Загружаем следующее предложение по индексу:", state.currentSentenceIndex);
             loadSentence();
        }
        console.log("--- handleNextSentence End ---");
    }
    
    function handlePlayAudio() {
        if (state.currentAudio) { alert("Аудио пока не работает.\nПуть: " + state.currentAudio); }
    }


    async function checkAndCompleteLesson() {
        console.log('--- checkAndCompleteLesson: НАЧАЛО ---');
        if (!state.currentLessonId) {
            console.error("checkAndCompleteLesson: currentLessonId не установлен!");
            showView('dashboard');
            return;
        }
    
        try {
            // 1. Получаем САМЫЕ СВЕЖИЕ данные о прогрессе
            const freshSentences = await fetchLessonSentencesData(state.currentLessonId);
            if (!freshSentences) {
                console.error("checkAndCompleteLesson: Не удалось получить свежие данные.");
                showView('dashboard');
                return;
            }
    
            // 2. Обновляем локальное состояние
            state.sentences = freshSentences;
            console.log('--- checkAndCompleteLesson: Получены свежие данные, предложений:', state.sentences.length);
    
            // 3. Ищем ПЕРВОЕ предложение, которое НЕ 'mastered'
            // (Используем ту же логику, что и в fetchSentences)
            const nextSentenceIndex = state.sentences.findIndex(s => !(s.status?.Valid && s.status.String === 'mastered'));
    
            console.log('--- checkAndCompleteLesson: Результат findIndex (ищем НЕ mastered):', nextSentenceIndex);
    
            if (nextSentenceIndex !== -1) {
                // Ошибки еще есть!
                console.log(`--- checkAndCompleteLesson: Найдены ошибки. Запускаем повтор с индекса ${nextSentenceIndex}...`);
                alert('Отлично! Теперь повторим ошибки.');
                
                // Переключаемся на это предложение
                state.currentSentenceIndex = nextSentenceIndex;
                loadSentence();
            } else {
                // Ошибок нет, все 'mastered'
                console.log('--- checkAndCompleteLesson: Все mastered. Урок пройден!');
                alert('Урок пройден!');
                showView('dashboard'); // Возвращаем на дэшборд
            }
        } catch (error) {
            console.error('Критическая ошибка в checkAndCompleteLesson:', error);
            showView('dashboard');
        }
    }




    // === Функции Переключения Видов ===
    function showView(viewName) {
         console.log("Переключение на view:", viewName); Object.values(views).forEach(view => { if(view) view.style.display = "none"; }); if (views[viewName]) { views[viewName].style.display = "block"; } else { console.error("Попытка показать неизвестный view:", viewName); if (window.location.pathname.startsWith('/app') && views.dashboard) showView('dashboard'); else if (views.auth) showView('auth'); }
    }
    function showAuthView(formName) {
        if(loginError) loginError.style.display = "none"; if(registerError) registerError.style.display = "none"; if(loginForm) loginForm.style.display = (formName === 'login') ? "block" : "none"; if(registerForm) registerForm.style.display = (formName === 'register') ? "block" : "none";
    }

    // --- Функции Модалки ---
    function openAuthModal(defaultForm = 'login') {
        if (!authOverlay || !authModal) { console.error("Элементы модального окна не найдены!"); return; }
        authOverlay.style.display = "block"; authModal.style.display = "block";
        document.body.classList.add("modal-open"); showAuthView(defaultForm);
        const firstInput = (defaultForm === 'login') ? document.getElementById("login-email") : document.getElementById("register-email");
        if(firstInput) setTimeout(() => firstInput.focus(), 50);
    }
    function closeAuthModal() {
        if (!authOverlay || !authModal) return;
        authOverlay.style.display = "none"; authModal.style.display = "none";
        document.body.classList.remove("modal-open");
    }
    // ---------------------------------

    // === Привязка Событий ===
    if (loginForm) loginForm.addEventListener("submit", handleLogin);
    if (registerForm) registerForm.addEventListener("submit", handleRegister);
    if (showRegisterBtn) showRegisterBtn.addEventListener("click", (e) => { e.preventDefault(); showAuthView('register'); });
    if (showLoginBtn) showLoginBtn.addEventListener("click", (e) => { e.preventDefault(); showAuthView('login'); });
    const handleStartClick = (e) => { e.preventDefault(); openAuthModal('login'); };
    if (headerStartButton) { headerStartButton.addEventListener("click", handleStartClick); } else { console.log("Кнопка header-start-button не найдена."); }
    if (heroStartButton) { heroStartButton.addEventListener("click", handleStartClick); } else { console.log("Кнопка hero-start-button не найдена."); }
    if (closeModalButton) { closeModalButton.addEventListener("click", closeAuthModal); }
    if (authOverlay) { authOverlay.addEventListener("click", closeAuthModal); }
    if (logoutButton) logoutButton.addEventListener("click", handleLogout);
    if (levelsContainer) levelsContainer.addEventListener("click", (e) => { const levelItem = e.target.closest(".level-item"); if (levelItem && levelsContainer) { levelsContainer.querySelectorAll(".level-item").forEach(el => el.classList.remove("active")); levelItem.classList.add("active"); fetchLessons(levelItem.dataset.id); } });
    if (lessonsContainer) lessonsContainer.addEventListener("click", (e) => { const lessonItem = e.target.closest(".lesson-item"); if (lessonItem) { fetchSentences(lessonItem.dataset.id); } });
    if (checkAnswerBtn) checkAnswerBtn.addEventListener("click", handleCheckAnswer);
    if (nextSentenceBtn) nextSentenceBtn.addEventListener("click", handleNextSentence);
    if (backToLessonsBtn) backToLessonsBtn.addEventListener("click", () => showView('dashboard'));
    if (playAudioBtn) playAudioBtn.addEventListener("click", handlePlayAudio);
    if (userAnswer) userAnswer.addEventListener("keydown", (e) => { if (e.code === 'Enter' && !userAnswer.disabled && checkAnswerBtn && checkAnswerBtn.style.display === "block") { handleCheckAnswer(); e.preventDefault(); e.stopPropagation(); } });
    document.addEventListener('keydown', function(event) { if (event.code === 'Enter' && nextSentenceBtn && nextSentenceBtn.style.display === 'block') { if (document.activeElement !== userAnswer || (userAnswer && userAnswer.disabled)) { handleNextSentence(); event.preventDefault(); } } if (event.key === 'Escape' && authModal && authModal.style.display === 'block') { closeAuthModal(); } });

    // === ИНИЦИАЛИЗАЦИЯ ===
    const currentPath = window.location.pathname;
    console.log("Инициализация JS. Текущий путь:", currentPath);
    if (currentPath === '/app' || currentPath.startsWith('/app/')) {
        console.log("На странице /app");
        if(state.token) { if(logoutButton) logoutButton.style.display = "block"; else console.error("Logout button not found on /app"); fetchLevels(); showView('dashboard'); }
        else { console.log("Нет токена на /app, редирект на /"); window.location.href = "/"; }
    } else if (currentPath === '/') {
        console.log("На странице /"); if(logoutButton) logoutButton.style.display = "none";
        closeAuthModal();
    } else { console.warn("Неизвестный путь:", currentPath, "- редирект на /"); window.location.href = "/"; }

}); // Конец DOMContentLoaded

    
