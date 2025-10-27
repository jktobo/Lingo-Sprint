package api

import (
	"context"
	"errors" // <-- ДОБАВЛЕНО
	// "fmt" // <-- УБРАНО
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// UserIDKey - это ключ, который мы будем использовать для хранения ID пользователя в "контексте" запроса.
type UserIDKey string
const ContextUserIDKey UserIDKey = "userID"

// AuthMiddleware - наш "охранник"
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Получаем заголовок "Authorization"
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondWithError(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		// 2. Проверяем, что он начинается с "Bearer "
		headerParts := strings.Split(authHeader, " ")
		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			respondWithError(w, http.StatusUnauthorized, "Invalid Authorization header format")
			return
		}

		tokenString := headerParts[1]
		claims := &Claims{} // Используем структуру Claims из handlers.go

		// 3. Парсим токен, используя наш секретный ключ (jwtKey из handlers.go)
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) { // <-- Теперь 'errors' определен
				respondWithError(w, http.StatusUnauthorized, "Token has expired")
			} else {
				respondWithError(w, http.StatusUnauthorized, "Invalid token")
			}
			return
		}

		if !token.Valid {
			respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// 4. УСПЕХ! Токен валидный. "Прикрепляем" ID пользователя к запросу.
		// Мы "обогащаем" запрос, добавляя в его "контекст" ID пользователя.
		ctx := context.WithValue(r.Context(), ContextUserIDKey, claims.UserID)
		
		// 5. Передаем "обогащенный" запрос следующему обработчику (например, GetLevels)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}