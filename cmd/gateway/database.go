package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// upsertUser crée ou met à jour un utilisateur dans la base de données
func (a *App) upsertUser(ctx context.Context, u twitchUser) (int64, error) {
	query := `
INSERT INTO users (twitch_user_id, login, display_name, avatar_url, created_at, updated_at)
VALUES (?, ?, ?, ?, NOW(6), NOW(6))
ON DUPLICATE KEY UPDATE
  login = VALUES(login),
  display_name = VALUES(display_name),
  avatar_url = VALUES(avatar_url),
  updated_at = NOW(6)
`
	res, err := a.db.ExecContext(ctx, query, u.ID, u.Login, u.DisplayName, u.ProfileImageURL)
	if err != nil {
		return 0, err
	}

	// Si user existait déjà, il faut le relire pour connaître son id interne
	if rowsAffected, _ := res.RowsAffected(); rowsAffected == 1 {
		// nouvel utilisateur inséré
		lastID, err := res.LastInsertId()
		if err == nil && lastID > 0 {
			return lastID, nil
		}
	}

	var id int64
	if err := a.db.QueryRowContext(ctx, "SELECT id FROM users WHERE twitch_user_id = ?", u.ID).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// createWebSession crée une nouvelle session web
func (a *App) createWebSession(ctx context.Context, userID int64, accessToken, refreshToken string, scopes []string) (string, error) {
	sessionID, err := randomHex(16)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour) // durée de session à ajuster

	scopeStr, err := json.Marshal(scopes)
	if err != nil {
		return "", err
	}

	query := `
INSERT INTO web_sessions (session_id, user_id, access_token, refresh_token, scopes, created_at, last_activity_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`
	_, err = a.db.ExecContext(ctx, query, sessionID, userID, accessToken, refreshToken, string(scopeStr), now, now, expiresAt)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

// setSessionCookie définit le cookie de session
func (a *App) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "tca_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // à passer à true en prod avec HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   24 * 60 * 60, // 24h
	})
}

// getSessionData récupère les données d'une session web
func (a *App) getSessionData(ctx context.Context, sessionID string) (*SessionData, error) {
	var (
		userID      int64
		accessToken string
	)
	err := a.db.QueryRowContext(ctx,
		`SELECT user_id, access_token FROM web_sessions WHERE session_id = ? AND expires_at > NOW(6)`,
		sessionID,
	).Scan(&userID, &accessToken)
	if err != nil {
		return nil, err
	}
	return &SessionData{
		SessionID:   sessionID,
		UserID:      userID,
		AccessToken: accessToken,
	}, nil
}

// getOrCreateAnalysisSession récupère ou crée une session d'analyse active
func (a *App) getOrCreateAnalysisSession(ctx context.Context, userID int64) (int64, string, error) {
	// Tenter de trouver une session active existante
	var (
		id          int64
		sessionUUID string
	)
	err := a.db.QueryRowContext(ctx,
		`SELECT id, session_uuid FROM sessions WHERE user_id = ? AND status = 'active' LIMIT 1`,
		userID,
	).Scan(&id, &sessionUUID)
	if err == nil {
		return id, sessionUUID, nil
	}
	if err != sql.ErrNoRows {
		return 0, "", err
	}

	// Créer une nouvelle session
	sessionUUID, err = randomHex(16)
	if err != nil {
		return 0, "", err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)

	res, err := a.db.ExecContext(ctx,
		`INSERT INTO sessions (session_uuid, user_id, status, created_at, expires_at, updated_at)
         VALUES (?, ?, 'active', ?, ?, ?)`,
		sessionUUID, userID, now, expiresAt, now,
	)
	if err != nil {
		return 0, "", err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	return newID, sessionUUID, nil
}

// getActiveSessionUUID récupère l'UUID de la session active d'un utilisateur
func (a *App) getActiveSessionUUID(ctx context.Context, userID int64) (string, error) {
	var sessionUUID string
	err := a.db.QueryRowContext(ctx,
		`SELECT session_uuid FROM sessions WHERE user_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&sessionUUID)
	if err != nil {
		return "", err
	}
	return sessionUUID, nil
}
