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
    const progressBar = document.getElementById("progress-bar-inner");
    const progressText = document.getElementById("progress-text");

    // === Состояние приложения ===
    let state = {
        token: localStorage.getItem("token") || null,
        levels: [],
        lessons: [],
        sentences: [],
        currentSentenceIndex: 0,
        displayProgressNumber: 0,
        currentAudio: null,
    };

    // === API-функции ===
    async function fetchProtected(url, options = {}) {
        if (!state.token) {
            console.error("No token found, logging out.");
            handleLogout(); // Ensure user is logged out if token is missing
            throw new Error("Missing authentication token"); // Stop execution
        }
        const defaultHeaders = {
            'Authorization': `Bearer ${state.token}`,
            'Content-Type': 'application/json'
        };
        const config = { ...options, headers: { ...defaultHeaders, ...options.headers } };

        try {
            const response = await fetch(url, config);
            if (response.status === 401) {
                console.error("Token invalid or expired, logging out.");
                handleLogout();
                throw new Error("Unauthorized"); // Stop execution
            }
            if (!response.ok) {
                // Handle other non-401 errors if needed
                console.error(`API Error ${response.status}: ${response.statusText}`);
                throw new Error(`API Error: ${response.status}`);
            }
            return response;
        } catch (error) {
            // Log network errors or errors thrown above
            console.error("FetchProtected error:", error);
            // Optionally, inform the user about network issues
            // alert("Network error. Please try again.");
            throw error; // Re-throw to stop subsequent actions if critical
        }
    }


    async function fetchLevels() {
        try {
            const response = await fetchProtected("/api/levels");
            state.levels = await response.json();
            renderLevels();
        } catch (error) { console.error("Failed to fetch levels:", error); }
    }

    async function fetchLessons(levelId) {
        try {
            const response = await fetchProtected(`/api/levels/${levelId}/lessons`);
            state.lessons = await response.json();
            renderLessons();
        } catch (error) { console.error("Failed to fetch lessons:", error); }
    }

    async function fetchSentences(lessonId) {
        try {
            const response = await fetchProtected(`/api/lessons/${lessonId}/sentences`);
            if (!response) return; // Stop if fetchProtected failed (e.g., logged out)
            state.sentences = await response.json();

            const firstUnansweredIndex = state.sentences.findIndex(s =>
                !(s.status.Valid && s.status.String === 'mastered')
            );

            if (firstUnansweredIndex === -1 && state.sentences.length > 0) {
                 alert("Вы уже прошли этот урок! Показываем заново.");
                state.currentSentenceIndex = 0;
                state.displayProgressNumber = 1;
            } else {
                state.currentSentenceIndex = (firstUnansweredIndex === -1) ? 0 : firstUnansweredIndex;
                let masteredCount = 0;
                for (let i = 0; i < state.currentSentenceIndex; i++) {
                    if (state.sentences[i].status.Valid && state.sentences[i].status.String === 'mastered') {
                        masteredCount++;
                    }
                }
                state.displayProgressNumber = masteredCount + 1;
            }

            if (state.sentences.length > 0) {
                showView('trainer');
                loadSentence();
            } else {
                alert("В этом уроке нет предложений.");
                showView('dashboard');
            }
        } catch (error) { console.error("Failed to fetch sentences:", error); }
    }

     async function saveProgress(sentenceId, isCorrect) {
        try {
            await fetchProtected("/api/progress/save", {
                method: "POST",
                body: JSON.stringify({ sentence_id: sentenceId, is_correct: isCorrect })
            });
            console.log(`Progress saved for sentence ${sentenceId}: ${isCorrect}`);
        } catch (error) { console.error("Failed to save progress:", error); }
    }


    // === Функции Аутентификации ===
    async function handleLogin(e) {
        e.preventDefault();
        loginError.style.display = "none"; // Hide previous error
        const email = document.getElementById("login-email").value;
        const password = document.getElementById("login-password").value;
        try {
            const response = await fetch("/api/login", {
                method: "POST", headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || "Ошибка входа");
            saveToken(data.token);
            checkAuth();
        } catch (error) {
            loginError.textContent = error.message;
            loginError.style.display = "block";
        }
    }

    async function handleRegister(e) {
        e.preventDefault();
        registerError.style.display = "none"; // Hide previous error
        const email = document.getElementById("register-email").value;
        const password = document.getElementById("register-password").value;
         if (password.length < 6) { // Basic validation
             registerError.textContent = "Пароль должен быть не менее 6 символов.";
             registerError.style.display = "block";
             return;
         }
        try {
            const response = await fetch("/api/register", {
                method: "POST", headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || "Ошибка регистрации");
            alert("Регистрация успешна! Теперь вы можете войти.");
            showAuthView('login');
            registerForm.reset();
        } catch (error) {
            registerError.textContent = error.message;
            registerError.style.display = "block";
        }
    }

    function handleLogout() {
        localStorage.removeItem("token");
        state.token = null;
        // Сброс состояния уроков/предложений при выходе
        state.levels = [];
        state.lessons = [];
        state.sentences = [];
        state.currentSentenceIndex = 0;
        state.displayProgressNumber = 0;
        checkAuth();
    }

    function saveToken(token) {
        localStorage.setItem("token", token);
        state.token = token;
    }

    function checkAuth() {
        if (state.token) {
            showView('dashboard');
            // Only fetch levels if they haven't been loaded yet
            if (state.levels.length === 0) {
                 fetchLevels();
            }
            logoutButton.style.display = "block";
        } else {
            showView('auth');
             showAuthView('login'); // Ensure login form is shown by default
            logoutButton.style.display = "none";
        }
    }

    // === Функции Рендеринга ===
    function renderLevels() {
        levelsContainer.innerHTML = "";
        state.levels.forEach(level => {
            const el = document.createElement("div");
            el.className = "grid-item level-item";
            el.textContent = level.title; el.dataset.id = level.id;
            levelsContainer.appendChild(el);
        });
    }

    function renderLessons() {
        lessonsContainer.innerHTML = "";
        state.lessons.forEach(lesson => {
            const el = document.createElement("div");
            el.className = "grid-item lesson-item";
            el.textContent = `Урок ${lesson.lesson_number}`;
            el.dataset.id = lesson.id; el.dataset.title = lesson.title;
            lessonsContainer.appendChild(el);
        });
    }

    function loadSentence() {
        // Safety check: ensure sentences are loaded and index is valid
        if (!state.sentences || state.currentSentenceIndex >= state.sentences.length) {
             console.log("Attempted to load sentence beyond array bounds or before sentences loaded.");
             showView('dashboard'); // Go back if something is wrong
             return;
        }

        const sentence = state.sentences[state.currentSentenceIndex];
        // Safety check for sentence object
        if (!sentence) {
            console.error("Error: sentence object is undefined at index", state.currentSentenceIndex);
            showView('dashboard');
            return;
        }

        const lessonButton = document.querySelector(`.lesson-item[data-id='${sentence.lesson_id}']`);
        lessonTitle.textContent = lessonButton ? lessonButton.dataset.title : "Загрузка...";
        promptRu.textContent = sentence.prompt_ru || "[Русский текст не загружен]"; // Fallback text
        state.currentAudio = sentence.audio_path;

        userAnswer.value = "";
        userAnswer.disabled = false;
        checkAnswerBtn.style.display = "block";
        feedback.style.display = "none";
        nextSentenceBtn.style.display = "none";

        const totalNum = state.sentences.length;
        const barProgress = ((state.currentSentenceIndex + 1) / totalNum) * 100;
        progressBar.style.width = `${barProgress}%`;
        // Ensure displayProgressNumber is at least 1 when showing the trainer
        progressText.textContent = `${Math.max(1, state.displayProgressNumber)} / ${totalNum}`;


        userAnswer.focus();
    }

    // === Функции-Обработчики ===
    function handleCheckAnswer() {
        // Safety check: ensure sentences are loaded and index is valid
        if (!state.sentences || state.currentSentenceIndex >= state.sentences.length) {
            console.error("Error: Tried to check answer but sentences/index is invalid.");
            return;
        }
        const user = userAnswer.value.trim();
        const currentSentence = state.sentences[state.currentSentenceIndex];
        // Safety check for currentSentence
        if (!currentSentence || typeof currentSentence.answer_en === 'undefined') {
             console.error("Error: currentSentence or answer_en is undefined in handleCheckAnswer.");
             return;
        }

        const correct = currentSentence.answer_en.trim();
        const isCorrect = user.toLowerCase() === correct.toLowerCase();

        feedback.style.display = "block";
        feedbackText.textContent = isCorrect ? "Правильно! 👍" : "Ошибка 😞";
        correctAnswer.textContent = correct;
        feedback.className = isCorrect ? "correct" : "incorrect";

        saveProgress(currentSentence.id, isCorrect);

        // Store correctness for the 'Next' action
        nextSentenceBtn.dataset.wasCorrect = isCorrect.toString(); // Store as string 'true' or 'false'

        userAnswer.disabled = true;
        checkAnswerBtn.style.display = "none";
        nextSentenceBtn.style.display = "block";
        nextSentenceBtn.focus(); // Focus on the "Next" button for Enter key
    }

    function handleNextSentence() {
        const wasCorrect = nextSentenceBtn.dataset.wasCorrect === 'true'; // Convert back to boolean
        // Only increment display number if correct AND it's not already the last number
        if (wasCorrect && state.displayProgressNumber < state.sentences.length) {
            state.displayProgressNumber++;
        }
        state.currentSentenceIndex++;
        // Check if we finished the lesson *after* incrementing index
        if (state.currentSentenceIndex >= state.sentences.length) {
             alert("Урок пройден!");
             showView('dashboard');
        } else {
             loadSentence(); // Load next sentence
        }
    }

    function handlePlayAudio() {
        if (state.currentAudio) {
            alert("Воспроизведение аудио будет доступно после загрузки файлов в хранилище. \nПуть: " + state.currentAudio);
            // const audio = new Audio(`/audio/${state.currentAudio}`);
            // audio.play().catch(e => console.error("Ошибка воспроизведения:", e));
        }
    }

    // === Функции Переключения Видов ===
    function showView(viewName) {
        Object.values(views).forEach(view => view.style.display = "none");
        if (views[viewName]) views[viewName].style.display = "block";
        else console.error("Tried to show unknown view:", viewName); // Debugging
    }

    function showAuthView(formName) {
        loginError.style.display = "none";
        registerError.style.display = "none";
        loginForm.style.display = (formName === 'login') ? "block" : "none";
        registerForm.style.display = (formName === 'register') ? "block" : "none";
    }

    // === Привязка Событий ===
    loginForm.addEventListener("submit", handleLogin);
    registerForm.addEventListener("submit", handleRegister);
    logoutButton.addEventListener("click", handleLogout);
    showRegisterBtn.addEventListener("click", (e) => { e.preventDefault(); showAuthView('register'); });
    showLoginBtn.addEventListener("click", (e) => { e.preventDefault(); showAuthView('login'); });

    levelsContainer.addEventListener("click", (e) => {
        if (e.target.classList.contains("level-item")) {
            document.querySelectorAll(".level-item").forEach(el => el.classList.remove("active"));
            e.target.classList.add("active");
            fetchLessons(e.target.dataset.id);
        }
    });

    lessonsContainer.addEventListener("click", (e) => {
        if (e.target.classList.contains("lesson-item")) {
            fetchSentences(e.target.dataset.id);
        }
    });

    checkAnswerBtn.addEventListener("click", handleCheckAnswer);
    nextSentenceBtn.addEventListener("click", handleNextSentence);
    backToLessonsBtn.addEventListener("click", () => showView('dashboard'));
    playAudioBtn.addEventListener("click", handlePlayAudio);

    // --- ИСПРАВЛЕННЫЕ ОБРАБОТЧИКИ ENTER ---
    userAnswer.addEventListener("keydown", (e) => {
        // Check if Enter is pressed, the check button is visible,
        // AND the textarea is currently enabled (not disabled after checking)
        if (e.key === "Enter" && !userAnswer.disabled && checkAnswerBtn.style.display === "block") {
            handleCheckAnswer();
            e.preventDefault(); // Prevent newline
            e.stopPropagation(); // Prevent document listener
        }
    });

    document.addEventListener('keydown', function(event) {
        // Check if Enter is pressed AND the next button is visible
        if (event.key === 'Enter' && nextSentenceBtn.style.display === 'block') {
             // Check if the focus is NOT on the textarea (to avoid double trigger)
             // Or if the textarea IS focused but DISABLED (meaning we just checked)
             if (document.activeElement !== userAnswer || userAnswer.disabled) {
                 handleNextSentence();
                 event.preventDefault(); // Prevent potential default actions
             }
        }
    });
    // ------------------------------------

    // === ИНИЦИАЛИЗАЦИЯ ===
    checkAuth(); // Check authentication status on page load
});