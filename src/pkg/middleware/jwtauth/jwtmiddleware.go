package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const AuthContextKey contextKey = "auth"

type UserClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

type AuthContext struct {
	IsValid bool        `json:"is_valid"`
	Claims  *UserClaims `json:"claims,omitempty"`
	UserID  int64       `json:"user_id,omitempty"`
}

func AuthMiddleware(secretKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := AuthContext{
				IsValid: false,
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				ctx := context.WithValue(r.Context(), AuthContextKey, authCtx)
				ctx = context.WithValue(ctx, "userID", int64(0))
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				ctx := context.WithValue(r.Context(), AuthContextKey, authCtx)
				ctx = context.WithValue(ctx, "userID", int64(0))
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			tokenString := parts[1]
			claims := &UserClaims{}

			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(secretKey), nil
			})

			if err == nil && token.Valid {
				authCtx.IsValid = true
				authCtx.Claims = claims
				authCtx.UserID = claims.UserID
			}

			ctx := context.WithValue(r.Context(), AuthContextKey, authCtx)
			// Дублируем userID для обратной совместимости с хэндлерами
			ctx = context.WithValue(ctx, "userID", authCtx.UserID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
