package mcp

import (
	"crypto/subtle"
	"net/http"
)

func AuthMiddleware(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		want := "Bearer " + apiKey
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
