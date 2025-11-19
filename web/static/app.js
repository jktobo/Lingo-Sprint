/*
======================================================
LINGO-SPRINT APP.JS (ФИНАЛЬНЫЙ ФИКС: ЗВЕЗДЫ ПОЛНОСТЬЮ)
======================================================
*/

document.addEventListener("DOMContentLoaded", () => {
    console.log("App started...");

    // === 1. Состояние приложения ===
    let state = {
        token: localStorage.getItem("token") || null,
        userEmail: localStorage.getItem("userEmail") || null,
        currentView: 'dashboard',
        currentLessonId: null,
        levels: [],
        lessons: [],
        sentences: [],
        currentSentenceIndex: 0,
        isPremium: false,
    };

    // === 2. Элементы DOM ===
    const getDom = () => ({
        views: {
            auth: document.getElementById("auth-view"),
            dashboard: document.getElementById("dashboard-view"),
            trainer: document.getElementById("trainer-view"),
            subscription: document.getElementById("subscription-view"),
        },
        navItems: document.querySelectorAll(".nav-item"),
        userName: document.querySelector(".user-profile .user-name"),
        userAvatar: document.querySelector(".user-avatar img"),
        logoutButton: document.getElementById("logout-button"),
        authFormLogin: document.getElementById("login-form"),
        authFormRegister: document.getElementById("register-form"),
        showRegisterBtn: document.getElementById("show-register"),
        showLoginBtn: document.getElementById("show-login"),
        header: document.querySelector(".top-header"),
        mainContent: document.querySelector(".content"),
        levelsContainer: document.getElementById("levels-container"),
        lessonsContainer: document.getElementById("lessons-container"),
        currentLevelTitle: document.getElementById("current-level-title"),
        lessonProgressText: document.getElementById("lesson-progress-text"),
        
        statLessonsCompleted: document.getElementById("stat-lessons-completed"),
        statStars: document.getElementById("stat-stars"),
        statTime: document.getElementById("stat-time"),
        statAccuracy: document.getElementById("stat-accuracy"),
        
        globalProgressTitle: document.getElementById("global-progress-title"),
        globalProgressSubtitle: document.getElementById("global-progress-subtitle"),
        globalProgressBarInner: document.getElementById("global-progress-bar-inner"),
        globalProgressPercentText: document.getElementById("global-progress-percent-text"),
        
        trainerCard: document.getElementById("card"),
        backToLessonsBtn: document.getElementById("back-to-lessons"),
        progressText: document.getElementById("progress-text"),
        progressBarInner: document.getElementById("progress-bar-inner"),
        playAudioBtn: document.getElementById("play-audio"),
        promptRu: document.getElementById("current-sentence-ru"),
        userAnswer: document.getElementById("user-answer"),
        feedback: document.getElementById("feedback"),
        aiExplanation: document.getElementById("ai-explanation"),
        checkAnswerBtn: document.getElementById("check-answer"),
        nextSentenceBtn: document.getElementById("next-sentence"),
        
        premiumModalOverlay: document.getElementById("premium-modal-overlay"),
        premiumModalCloseBtn: document.querySelector("#premium-modal-overlay .modal-close"),
        premiumModalBuyBtn: document.getElementById("modal-buy-premium"),
    });

    let dom = getDom();

    // === 3. Вспомогательные функции ===
    function pluralizeLesson(count) {
        const cases = [2, 0, 1, 1, 1, 2];
        const titles = ['урок', 'урока', 'уроков'];
        const index = (count % 100 > 4 && count % 100 < 20) ? 2 : cases[(count % 10 < 5) ? count % 10 : 5];
        return titles[index];
    }
    function isSentenceMastered(sentence) {
        if (!sentence.status) return false;
        if (typeof sentence.status === 'object' && sentence.status.Valid) {
            return sentence.status.String === 'mastered';
        }
        return sentence.status === 'mastered';
    }

    // === 4. API Функции ===

    async function fetchProtected(url, options = {}) {
        if (!state.token) { handleLogout(); return undefined; }
        const defaultHeaders = { 'Authorization': `Bearer ${state.token}`, 'Content-Type': 'application/json' };
        const config = { ...options, headers: { ...defaultHeaders, ...options.headers } };

        try {
            const response = await fetch(url, config);
            if (response.status === 401) { handleLogout(); return undefined; }
            if (response.status === 403) { return response; }
            if (!response.ok) throw new Error(`API Error: ${response.status}`);
            return response;
        } catch (error) {
            console.error("Network error:", error);
            throw error;
        }
    }

    async function fetchLevels() {
        try {
            const response = await fetchProtected(`/api/levels?t=${Date.now()}`);
            if (!response) return;
            
            const data = await response.json(); 
            state.levels = data.levels; 

            const completed = data.completed_lessons || 0;
            const total = data.total_lessons || 0;
            let globalPercent = 0;
            if (total > 0) {
                globalPercent = (completed / total) * 100;
            }

            if (dom.statLessonsCompleted) dom.statLessonsCompleted.textContent = `${completed}/${total}`;
            if (dom.statTime) {
                const hours = data.study_time_hours || 0;
                dom.statTime.textContent = `${hours.toFixed(1)}ч`;
            }
            
            if (dom.statAccuracy) {
                const accuracy = data.accuracy || 0;
                dom.statAccuracy.textContent = `${accuracy.toFixed(0)}%`;
            }
            
            // ▼▼▼ Обновление глобальных звезд (С БЭКЕНДА) ▼▼▼
            if (dom.statStars) {
                const earned = data.earned_stars || 0;
                const totalPossible = data.total_stars || 0;
                dom.statStars.textContent = `${earned}/${totalPossible}`;
            }
            // ▲▲▲ Конец обновления ▼▼▼
            
            if (dom.globalProgressTitle) dom.globalProgressTitle.textContent = completed === 0 ? "Начните свой путь!" : "Продолжайте в том же духе!";
            if (dom.globalProgressSubtitle) {
                const remaining = total - completed;
                dom.globalProgressSubtitle.textContent = `Вы прошли ${completed} ${pluralizeLesson(completed)}. Осталось ${remaining} ${pluralizeLesson(remaining)}!`;
            }
            if (dom.globalProgressBarInner) dom.globalProgressBarInner.style.width = `${globalPercent.toFixed(0)}%`;
            if (dom.globalProgressPercentText) dom.globalProgressPercentText.textContent = `${globalPercent.toFixed(0)}% до финиша`;

            renderLevels();
            
            if (state.levels && state.levels.length > 0) {
                const activeBtn = dom.levelsContainer.querySelector('.level-item.active');
                if (!activeBtn) {
                    const firstLevel = state.levels.find(l => l.title === "A0") || state.levels[0];
                    updateActiveLevelUI(firstLevel.id);
                    fetchLessons(firstLevel.id);
                }
            }
        } catch (error) {
            console.error("Error in fetchLevels:", error);
        }
    }

    async function fetchLessons(levelId) {
        if (!levelId) return;
        try {
            const response = await fetchProtected(`/api/levels/${levelId}/lessons?t=${Date.now()}`);
            if (!response) return;
            state.lessons = await response.json();
            renderLessons();
        } catch (error) {
            console.error("Error in fetchLessons:", error);
        }
    }

    async function fetchSentences(lessonId) {
        if (!lessonId) return;
        state.currentLessonId = lessonId;

        try {
            const response = await fetchProtected(`/api/lessons/${lessonId}/sentences?t=${Date.now()}`);
            if (!response) return;

            if (response.status === 403) {
                showPremiumModal();
                return;
            }

            state.sentences = await response.json();
            
            if (!state.sentences || state.sentences.length === 0) {
                alert("В этом уроке пока нет предложений.");
                return;
            }

            const firstUnfinished = state.sentences.findIndex(s => !isSentenceMastered(s));
            
            if (firstUnfinished === -1) {
                if(confirm("Вы уже прошли этот урок. Хотите повторить?")) {
                    state.currentSentenceIndex = 0; 
                } else {
                    showView('dashboard');
                    return;
                }
            } else {
                state.currentSentenceIndex = firstUnfinished;
            }

            showView('trainer');
            loadSentence();

        } catch (error) {
            console.error("Error in fetchSentences:", error);
        }
    }

    async function saveProgress(sentenceId, isCorrect) {
        try {
            const response = await fetchProtected("/api/progress/save", {
                method: "POST",
                body: JSON.stringify({ sentence_id: sentenceId, is_correct: isCorrect })
            });
            
            if (!response || !response.ok) {
                console.error("ОШИБКА СОХРАНЕНИЯ! Прогресс не записан в БД.");
                alert("Ошибка сервера при сохранении прогресса! Проверьте соединение или базу данных."); 
            }
        } catch (error) { 
            console.error("Error in saveProgress:", error); 
            alert("Ошибка сети при сохранении.");
        }
    }

    async function fetchErrorExplanation() {
        const sentence = state.sentences[state.currentSentenceIndex];
        const userAns = dom.userAnswer.value;
        if (!userAns.trim()) return;

        showAiExplanation("🤖 Анализирую...", 'loading');

        try {
            const response = await fetchProtected("/api/ai/explain-error", {
                method: "POST",
                body: JSON.stringify({
                    prompt_ru: sentence.prompt_ru,
                    correct_en: sentence.answer_en,
                    user_answer_en: userAns
                })
            });
            if (!response) return;
            const data = await response.json();
            if (data.error) showAiExplanation(data.error, 'error');
            else showAiExplanation(data.explanation, 'success');
        } catch (error) {
            showAiExplanation("Ошибка AI.", 'error');
        }
    }

    // === 5. UI Функции ===

    function showView(viewName) {
        state.currentView = viewName || 'dashboard';
        if (dom.views) Object.values(dom.views).forEach(v => v && v.classList.remove("active"));
        if (dom.views[state.currentView]) dom.views[state.currentView].classList.add("active");
        
        if (dom.navItems) {
            dom.navItems.forEach(item => {
                const isActive = item.dataset.view === state.currentView || (state.currentView === 'lessons' && item.dataset.view === 'dashboard');
                item.classList.toggle("active", isActive);
            });
        }
        
        if (state.currentView === 'dashboard') {
            resetTrainer();
            fetchLevels();
        }
    }

    function updateActiveLevelUI(levelId) {
        if (!dom.levelsContainer) return;
        dom.levelsContainer.querySelectorAll('.level-item').forEach(btn => {
            if (btn.dataset.id == levelId) btn.classList.add('active');
            else btn.classList.remove('active');
        });
    }

    function renderLevels() {
        if (!dom.levelsContainer) return;
        dom.levelsContainer.innerHTML = "";
        state.levels.forEach(level => {
            const btn = document.createElement("button");
            btn.className = "level-item";
            btn.textContent = getFullLevelName(level.title);
            btn.dataset.id = level.id;
            dom.levelsContainer.appendChild(btn);
        });
    }

    function getFullLevelName(t) {
        const map = {"A0":"A0 - Beginner","A1":"A1 - Elementary","A2":"A2 - Elementary","B1":"B1 - Intermediate","B2":"B2 - Upper-Intermediate","C1":"C1 - Advanced"};
        return map[t] || t;
    }

    function renderLessons() {
        if (!dom.lessonsContainer) return;
        dom.lessonsContainer.innerHTML = "";
        if (!state.lessons || state.lessons.length === 0) { 
            dom.lessonsContainer.innerHTML = "<p>Нет уроков</p>"; 
            return; 
        }
        
        const activeBtn = dom.levelsContainer.querySelector('.level-item.active');
        const levelTitle = activeBtn ? activeBtn.textContent.split(" - ")[0] : "Уровень";
        if (dom.currentLevelTitle) dom.currentLevelTitle.textContent = `Уроки уровня ${levelTitle}`;

        let completedCount = 0;

        state.lessons.forEach(lesson => {
            const total = lesson.total_sentences || 0;
            const completed = lesson.completed_sentences || 0;
            const percent = total > 0 ? (completed / total) * 100 : 0;
            if (percent === 100) completedCount++;

            const isFree = (lesson.lesson_number <= 5) || (levelTitle === "A0");
            const hasAccess = isFree || state.isPremium;

            // Звезды
            const stars = lesson.sentences_with_errors || 0; 
            
            // ▼▼▼ Логика "Серых звезд" ▼▼▼
            // Если урок не завершен (percent < 100), звезды серые. Иначе золотые.
            const starsClass = (percent === 100) ? '' : 'gray';
            
            // Если не завершен, показываем 3 серые звезды, или 0 (по вашему выбору)
            let currentStarsHtml = '';

            if (percent === 100) {
                // Если урок пройден: генерируем активные и неактивные звезды
                for (let i = 0; i < 3; i++) {
                    if (i < stars) {
                        currentStarsHtml += '⭐'; // Активная звезда
                    } else {
                        currentStarsHtml += '<span class="empty-star">☆</span>'; // Серая неактивная звезда
                    }
                }
            } else {
                // Если урок не пройден: все три звезды серые (используя class="empty-star")
                currentStarsHtml = '<span class="empty-star">☆☆☆</span>';
            }

            let btnHtml;
            if (!hasAccess) btnHtml = `<button class="btn-orange" disabled>Разблокировать</button>`;
            else if (percent === 100) btnHtml = `<button class="btn-secondary">Повторить</button>`;
            else if (percent > 0) btnHtml = `<button class="btn-primary">Продолжить</button>`;
            else btnHtml = `<button class="btn-primary">Начать</button>`;

            let countText = `${total} предложений`;
            if (total === 1) countText = `1 предложение`;
            else if (total > 1 && total < 5) countText = `${total} предложения`;

            const div = document.createElement("div");
            div.className = `lesson-card ${!hasAccess ? 'locked' : ''}`;
            div.dataset.id = lesson.id;
            div.innerHTML = `
                <div class="lesson-card-header">
                    <h4>${lesson.title}</h4>
                    ${!hasAccess ? '<span class="pro-badge">PRO</span>' : `<span class="stars ${starsClass}">${currentStarsHtml}</span>`}
                </div>
                <p>${countText} для изучения</p>
                <div class="progress-bar"><div class="progress-bar-inner" style="width: ${percent}%;"></div></div>
                <div class="card-actions">${btnHtml}</div>
            `;
            dom.lessonsContainer.appendChild(div);
        });
        
        if (dom.lessonProgressText) {
            dom.lessonProgressText.textContent = `${completedCount} из ${state.lessons.length} завершено`;
        }
    }

    function loadSentence() {
        if (!state.sentences || state.currentSentenceIndex >= state.sentences.length) {
             checkAndCompleteLesson();
             return;
        }
        const sentence = state.sentences[state.currentSentenceIndex];
        if (dom.promptRu) dom.promptRu.textContent = sentence.prompt_ru;
        if (dom.userAnswer) {
            dom.userAnswer.value = "";
            dom.userAnswer.disabled = false;
            dom.userAnswer.focus();
        }
        if (dom.feedback) dom.feedback.classList.add("hidden");
        if (dom.aiExplanation) dom.aiExplanation.classList.add("hidden");
        if (dom.checkAnswerBtn) dom.checkAnswerBtn.classList.remove("hidden");
        if (dom.nextSentenceBtn) dom.nextSentenceBtn.classList.add("hidden");

        if (dom.progressText && dom.progressBarInner) {
            const total = state.sentences.length;
            const current = state.currentSentenceIndex + 1;
            dom.progressText.textContent = `${current}/${total}`;
            dom.progressBarInner.style.width = `${(current / total) * 100}%`;
        }
    }

    function handleCheckAnswer() {
        const sentence = state.sentences[state.currentSentenceIndex];
        const userAns = dom.userAnswer.value.trim();
        const correctAns = sentence.answer_en.trim();
        const isCorrect = userAns.toLowerCase().replace(/[.,!?]/g, '') === correctAns.toLowerCase().replace(/[.,!?]/g, '');
        
        if (isCorrect) state.sentences[state.currentSentenceIndex].status = { String: 'mastered', Valid: true };
        else state.sentences[state.currentSentenceIndex].status = { String: 'learning', Valid: true };

        handlePlayAudio();
        showFeedback(isCorrect, correctAns);
        
        dom.userAnswer.disabled = true;
        dom.checkAnswerBtn.classList.add("hidden");
        dom.nextSentenceBtn.classList.remove("hidden");

        if (!isCorrect) fetchErrorExplanation();
        else dom.aiExplanation.classList.add("hidden");

        saveProgress(sentence.id, isCorrect);
        dom.nextSentenceBtn.focus();
    }

    function handleNextSentence() {
        let nextIndex = state.currentSentenceIndex + 1;
        while (nextIndex < state.sentences.length) {
            const nextS = state.sentences[nextIndex];
            if (!isSentenceMastered(nextS)) break;
            nextIndex++;
        }
        state.currentSentenceIndex = nextIndex;
        if (state.currentSentenceIndex >= state.sentences.length) checkAndCompleteLesson();
        else loadSentence();
    }

    function checkAndCompleteLesson() {
        const hasMistakes = state.sentences.some(s => !isSentenceMastered(s));
        if (hasMistakes) {
            alert("Урок завершен, но есть ошибки. Давайте их исправим! 🔄");
            state.currentSentenceIndex = 0;
            if (isSentenceMastered(state.sentences[0])) { handleNextSentence(); return; }
            loadSentence();
        } else {
            alert("Поздравляем! Урок полностью пройден! 🎉");
            showView('dashboard');
        }
    }

    function handlePlayAudio() {
        const sentence = state.sentences[state.currentSentenceIndex];
        if (!sentence) return;
        if (sentence.audio_path && sentence.audio_path.Valid) {
            const audio = new Audio(sentence.audio_path.String);
            audio.play().catch(() => playBrowserTTS(sentence.answer_en));
        } else {
            playBrowserTTS(sentence.answer_en);
        }
    }
    
    function playBrowserTTS(text) {
        if(!text) return;
        const u = new SpeechSynthesisUtterance(text);
        u.lang = "en-US";
        window.speechSynthesis.speak(u);
    }

    function showFeedback(isCorrect, ans) {
        dom.feedback.classList.remove("hidden", "correct", "incorrect");
        if (isCorrect) {
            dom.feedback.classList.add("correct");
            dom.feedback.innerHTML = "<strong>Правильно! 👍</strong>";
        } else {
            dom.feedback.classList.add("incorrect");
            dom.feedback.innerHTML = `<strong>Ошибка 😞</strong><br>Правильно: ${ans}`;
        }
    }
    
    function showAiExplanation(msg, type) {
        dom.aiExplanation.classList.remove("hidden");
        dom.aiExplanation.innerHTML = msg;
        dom.aiExplanation.style.opacity = type === 'loading' ? 0.6 : 1;
    }

    function showPremiumModal() {
        dom.premiumModalOverlay.classList.remove("hidden");
    }
    function hidePremiumModal() {
        dom.premiumModalOverlay.classList.add("hidden");
    }

    function handleLogout() {
        document.cookie = "auth_status=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
        localStorage.removeItem("token");
        localStorage.removeItem("userEmail");
        if (window.location.pathname.startsWith("/app")) window.location.href = "/";
        else {
             if (dom.header) dom.header.classList.add("hidden");
             if (dom.views.auth) dom.views.auth.classList.add("active");
        }
    }

    function resetTrainer() {
        state.currentLessonId = null;
        state.sentences = [];
        state.currentSentenceIndex = 0;
        if (dom.promptRu) dom.promptRu.textContent = "...";
        if (dom.userAnswer) dom.userAnswer.value = "";
        if (dom.progressText) dom.progressText.textContent = "0/0";
        if (dom.progressBarInner) dom.progressBarInner.style.width = "0%";
        if (dom.feedback) dom.feedback.classList.add("hidden");
        if (dom.aiExplanation) dom.aiExplanation.classList.add("hidden");
    }

    // === 6. Инициализация ===

    function init() {
        if (!dom.views.dashboard) return; 
        if (state.token) {
            if (dom.header) dom.header.classList.remove("hidden");
            if (dom.userName) dom.userName.textContent = state.userEmail;
            if (dom.userAvatar) dom.userAvatar.src = `https://ui-avatars.com/api/?name=${state.userEmail}&background=C026D3&color=fff`;
            fetchLevels();
            showView('dashboard');
        } else {
            handleLogout();
        }

        // Listeners
        if(dom.logoutButton) dom.logoutButton.addEventListener("click", handleLogout);
        if(dom.navItems) dom.navItems.forEach(item => item.addEventListener("click", (e) => {
            const view = e.currentTarget.dataset.view;
            if (view === 'lessons') {
                showView('dashboard');
                dom.mainContent.scrollTo({ top: dom.levelsContainer.offsetTop, behavior: 'smooth' });
            } else showView(view);
        }));
        if(dom.levelsContainer) dom.levelsContainer.addEventListener("click", (e) => {
            const btn = e.target.closest(".level-item");
            if (btn) {
                updateActiveLevelUI(btn.dataset.id);
                fetchLessons(btn.dataset.id);
            }
        });
        if(dom.lessonsContainer) dom.lessonsContainer.addEventListener("click", (e) => {
            const card = e.target.closest(".lesson-card");
            const btn = e.target.closest("button");
            if (!card || !btn) return;
            if (card.classList.contains("locked")) showPremiumModal();
            else fetchSentences(card.dataset.id);
        });

        if(dom.checkAnswerBtn) dom.checkAnswerBtn.addEventListener("click", handleCheckAnswer);
        if(dom.nextSentenceBtn) dom.nextSentenceBtn.addEventListener("click", handleNextSentence);
        if(dom.playAudioBtn) dom.playAudioBtn.addEventListener("click", handlePlayAudio);
        if(dom.backToLessonsBtn) dom.backToLessonsBtn.addEventListener("click", () => showView('dashboard'));
        
        if(dom.userAnswer) {
            dom.userAnswer.addEventListener("keydown", (e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    if (!dom.checkAnswerBtn.classList.contains("hidden")) handleCheckAnswer();
                    else if (!dom.nextSentenceBtn.classList.contains("hidden")) handleNextSentence();
                }
            });
        }
        if(dom.premiumModalCloseBtn) dom.premiumModalCloseBtn.addEventListener("click", hidePremiumModal);
        if(dom.premiumModalOverlay) dom.premiumModalOverlay.addEventListener("click", (e) => {
            if (e.target === dom.premiumModalOverlay) hidePremiumModal();
        });
        if (dom.authFormLogin) dom.authFormLogin.addEventListener("submit", handleLogin);
    }

    init();
});