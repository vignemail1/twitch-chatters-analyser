package main

import (
	"context"
	"log"
	"net/http"
	"time"
)

// loggingMiddleware logs HTTP requests with status code, method, path, and duration
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the ResponseWriter to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     200, // default status
			written:        false,
		}

		next.ServeHTTP(wrapped, r)

		log.Printf("%d %s %s from %s in %s",
			wrapped.statusCode,
			r.Method,
			r.URL.Path,
			r.RemoteAddr,
			time.Since(start),
		)
	})
}

// loadCurrentUser middleware charge l'utilisateur depuis la session
func (a *App) loadCurrentUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("tca_session")
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		sessionID := c.Value
		var (
			userID       int64
			twitchUserID string
			login        string
			displayName  string
		)

		// Jointure web_sessions -> users, vérifier que la session n'est pas expirée
		query := `
SELECT u.id, u.twitch_user_id, u.login, u.display_name
FROM web_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.session_id = ? AND s.expires_at > NOW(6)
LIMIT 1
`
		err = a.db.QueryRowContext(r.Context(), query, sessionID).Scan(&userID, &twitchUserID, &login, &displayName)
		if err != nil {
			// Session invalide/expirée : on ignore silencieusement
			next.ServeHTTP(w, r)
			return
		}

		// Mettre à jour last_activity_at
		_, _ = a.db.ExecContext(r.Context(), "UPDATE web_sessions SET last_activity_at = NOW(6) WHERE session_id = ?", sessionID)

		u := &CurrentUser{
			ID:           userID,
			TwitchUserID: twitchUserID,
			Login:        login,
			DisplayName:  displayName,
		}

		ctx := context.WithValue(r.Context(), ctxKeyUser, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
