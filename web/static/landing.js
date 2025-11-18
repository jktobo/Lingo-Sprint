/*
======================================================
JAVASCRIPT ТОЛЬКО ДЛЯ LENDING.HTML
======================================================
*/
document.addEventListener("DOMContentLoaded", () => {
    
    // === Элементы DOM Лендинга ===
    const dom = {
        // Кнопки открытия
        headerLoginBtn: document.getElementById("landing-login-button"),
        heroStartBtn: document.getElementById("landing-start-button"),
        ctaStartBtn: document.getElementById("landing-cta-start-button"),
        
        // Модалка
        overlay: document.getElementById("auth-overlay"),
        modal: document.getElementById("auth-modal"),
        closeModalBtn: document.getElementById("close-modal-button"),
        
        // Формы
        loginForm: document.getElementById("login-form"),
        registerForm: document.getElementById("register-form"),
        
        // Переключатели форм
        showRegisterLink: document.getElementById("show-register"),
        showLoginLink: document.getElementById("show-login"),
        
        // Ошибки
        loginError: document.getElementById("login-error"),
        registerError: document.getElementById("register-error"),
    };

    // === Функции Модального Окна ===
    function openAuthModal(formType = 'login') {
        if (!dom.overlay || !dom.modal) return;
        dom.overlay.style.display = "block";
        dom.modal.style.display = "block";
        showAuthForm(formType);
    }

    function closeAuthModal() {
        if (!dom.overlay || !dom.modal) return;
        dom.overlay.style.display = "none";
        dom.modal.style.display = "none";
    }

    function showAuthForm(formType) {
        if (dom.loginError) dom.loginError.style.display = "none";
        if (dom.registerError) dom.registerError.style.display = "none";

        if (formType === 'login') {
            if (dom.loginForm) dom.loginForm.style.display = "block";
            if (dom.registerForm) dom.registerForm.style.display = "none";
        } else {
            if (dom.loginForm) dom.loginForm.style.display = "none";
            if (dom.registerForm) dom.registerForm.style.display = "block";
        }
    }

    // === Функции Аутентификации ===
    async function handleLogin(e) {
        e.preventDefault();
        if (dom.loginError) dom.loginError.style.display = "none";
        
        const email = dom.loginForm.querySelector("#login-email").value;
        const password = dom.loginForm.querySelector("#login-password").value;

        try {
            const response = await fetch("/api/login", {
                method: "POST",
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || "Ошибка входа");

            // Сохраняем токен и email
            localStorage.setItem("token", data.token);
            localStorage.setItem("userEmail", email);
            
            // Устанавливаем cookie для Go-сервера
            document.cookie = "auth_status=logged_in; path=/; max-age=" + 60*60*24*3; 

            // Перенаправляем в приложение
            window.location.href = "/app"; 

        } catch (error) {
            if (dom.loginError) {
                dom.loginError.textContent = error.message;
                dom.loginError.style.display = "block";
            }
        }
    }

    async function handleRegister(e) {
        e.preventDefault();
        if (dom.registerError) dom.registerError.style.display = "none";
        
        const email = dom.registerForm.querySelector("#register-email").value;
        const password = dom.registerForm.querySelector("#register-password").value;

        if (password.length < 6) {
             if (dom.registerError) {
                dom.registerError.textContent = "Пароль должен быть не менее 6 символов.";
                dom.registerError.style.display = "block";
            }
            return;
        }

        try {
            const response = await fetch("/api/register", {
                method: "POST",
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password })
            });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || "Ошибка регистрации");

            // Успех
            alert("Регистрация успешна! Теперь войдите.");
            showAuthForm('login');

        } catch (error) {
            if (dom.registerError) {
                dom.registerError.textContent = error.message;
                dom.registerError.style.display = "block";
            }
        }
    }

    // === Привязка Событий ===
    
    // Открыть модалку (Вход)
    if (dom.headerLoginBtn) {
        dom.headerLoginBtn.addEventListener("click", (e) => {
            e.preventDefault();
            openAuthModal('login');
        });
    }
    
    // Открыть модалку (Регистрация)
    if (dom.heroStartBtn) {
        dom.heroStartBtn.addEventListener("click", (e) => {
            e.preventDefault();
            openAuthModal('register');
        });
    }
    if (dom.ctaStartBtn) {
        dom.ctaStartBtn.addEventListener("click", (e) => {
            e.preventDefault();
            openAuthModal('register');
        });
    }

    // Закрыть модалку
    if (dom.closeModalBtn) dom.closeModalBtn.addEventListener("click", closeAuthModal);
    if (dom.overlay) dom.overlay.addEventListener("click", closeAuthModal);

    // Переключение форм
    if (dom.showRegisterLink) {
        dom.showRegisterLink.addEventListener("click", (e) => {
            e.preventDefault();
            showAuthForm('register');
        });
    }
    if (dom.showLoginLink) {
        dom.showLoginLink.addEventListener("click", (e) => {
            e.preventDefault();
            showAuthForm('login');
        });
    }
    
    // Отправка форм
    if (dom.loginForm) dom.loginForm.addEventListener("submit", handleLogin);
    if (dom.registerForm) dom.registerForm.addEventListener("submit", handleRegister);

    // Анимации лендинга (fade-up)
    const io = new IntersectionObserver((entries) => {
        entries.forEach(e => {
            if (e.isIntersecting) {
                e.target.classList.add('in');
            }
        });
    }, { threshold: 0.08 });
    
    document.querySelectorAll('.fade-up').forEach(el => io.observe(el));
});