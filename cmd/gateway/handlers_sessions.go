package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// handleCreateCapture crée un job de capture de chatters
func (a *App) handleCreateCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	broadcasterID := r.Form.Get("broadcaster_id")
	broadcasterLogin := r.Form.Get("broadcaster_login")
	if broadcasterID == "" || broadcasterLogin == "" {
		http.Error(w, "missing broadcaster", http.StatusBadRequest)
		return
	}

	// Récupérer la session pour avoir le token
	c, err := r.Cookie("tca_session")
	if err != nil || c.Value == "" {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}
	sessData, err := a.getSessionData(r.Context(), c.Value)
	if err != nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Créer ou récupérer une session d'analyse
	sessionID, _, err := a.getOrCreateAnalysisSession(r.Context(), sessData.UserID)
	if err != nil {
		log.Printf("getOrCreateAnalysisSession error: %v", err)
		http.Error(w, "failed to create analysis session", http.StatusInternalServerError)
		return
	}

	// Créer un job FETCH_CHATTERS
	payload := map[string]interface{}{
		"session_id":        sessionID,
		"twitch_user_id":    u.TwitchUserID,
		"broadcaster_id":    broadcasterID,
		"broadcaster_login": broadcasterLogin,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal job payload error: %v", err)
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}

	_, err = a.db.ExecContext(r.Context(),
		`INSERT INTO jobs (type, payload, status, created_at) VALUES ('FETCH_CHATTERS', ?, 'pending', NOW(6))`,
		string(payloadJSON),
	)
	if err != nil {
		log.Printf("insert job error: %v", err)
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/channels?capture_enqueued=1", http.StatusFound)
}

// handleSaveSession marque une session active comme sauvegardée
func (a *App) handleSaveSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Récupérer la session active
	var sessionID int64
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM sessions WHERE user_id = ? AND status = 'active' LIMIT 1`,
		u.ID,
	).Scan(&sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Redirect(w, r, "/analysis?save_no_session=1", http.StatusFound)
			return
		}
		log.Printf("query session error: %v", err)
		http.Error(w, "failed to query session", http.StatusInternalServerError)
		return
	}

	// Marquer comme 'saved'
	_, err = a.db.ExecContext(r.Context(), `UPDATE sessions SET status = 'saved', updated_at = NOW(6) WHERE id = ?`, sessionID)
	if err != nil {
		log.Printf("update session error: %v", err)
		http.Error(w, "failed to save session", http.StatusInternalServerError)
		return
	}

	log.Printf("session %d saved by user %d", sessionID, u.ID)
	http.Redirect(w, r, "/sessions?saved=1", http.StatusFound)
}

// handlePurgeSession supprime une session active et ses données
func (a *App) handlePurgeSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Récupérer la session active
	var sessionID int64
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM sessions WHERE user_id = ? AND status = 'active' LIMIT 1`,
		u.ID,
	).Scan(&sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Redirect(w, r, "/channels?purge_no_session=1", http.StatusFound)
			return
		}
		log.Printf("query session error: %v", err)
		http.Error(w, "failed to query session", http.StatusInternalServerError)
		return
	}

	// Suppression en cascade : capture_chatters -> captures -> session
	_, err = a.db.ExecContext(r.Context(), `
DELETE cc FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
WHERE c.session_id = ?
`, sessionID)
	if err != nil {
		log.Printf("delete capture_chatters error: %v", err)
		http.Error(w, "failed to purge session", http.StatusInternalServerError)
		return
	}

	_, err = a.db.ExecContext(r.Context(), `DELETE FROM captures WHERE session_id = ?`, sessionID)
	if err != nil {
		log.Printf("delete captures error: %v", err)
		http.Error(w, "failed to purge session", http.StatusInternalServerError)
		return
	}

	_, err = a.db.ExecContext(r.Context(), `UPDATE sessions SET status = 'deleted', updated_at = NOW(6) WHERE id = ?`, sessionID)
	if err != nil {
		log.Printf("update session error: %v", err)
		http.Error(w, "failed to purge session", http.StatusInternalServerError)
		return
	}

	log.Printf("session %d purged by user %d", sessionID, u.ID)
	http.Redirect(w, r, "/channels?purged=1", http.StatusFound)
}

// handleDeleteSession supprime une session sauvegardée
func (a *App) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	sessionUUID := r.Form.Get("session_uuid")
	if sessionUUID == "" {
		http.Error(w, "missing session_uuid", http.StatusBadRequest)
		return
	}

	// Récupérer l'ID de la session
	var sessionID int64
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM sessions WHERE user_id = ? AND session_uuid = ? AND status = 'saved' LIMIT 1`,
		u.ID, sessionUUID,
	).Scan(&sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Redirect(w, r, "/sessions?delete_not_found=1", http.StatusFound)
			return
		}
		log.Printf("query session error: %v", err)
		http.Error(w, "failed to query session", http.StatusInternalServerError)
		return
	}

	// Suppression en cascade
	_, err = a.db.ExecContext(r.Context(), `
DELETE cc FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
WHERE c.session_id = ?
`, sessionID)
	if err != nil {
		log.Printf("delete capture_chatters error: %v", err)
		http.Error(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	_, err = a.db.ExecContext(r.Context(), `DELETE FROM captures WHERE session_id = ?`, sessionID)
	if err != nil {
		log.Printf("delete captures error: %v", err)
		http.Error(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	_, err = a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		log.Printf("delete session error: %v", err)
		http.Error(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	log.Printf("saved session %d deleted by user %d", sessionID, u.ID)
	http.Redirect(w, r, "/sessions?deleted=1", http.StatusFound)
}

// handleSessions affiche la liste des sessions sauvegardées
func (a *App) handleSessions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Récupérer toutes les sessions saved
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, session_uuid, status, created_at, updated_at 
         FROM sessions 
         WHERE user_id = ? AND status = 'saved' 
         ORDER BY updated_at DESC`,
		u.ID,
	)
	if err != nil {
		log.Printf("query sessions error: %v", err)
		http.Error(w, "failed to load sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sessions []SavedSession
	for rows.Next() {
		var s SavedSession
		if err := rows.Scan(&s.ID, &s.SessionUUID, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			log.Printf("scan session error: %v", err)
			continue
		}
		sessions = append(sessions, s)
	}

	// Vérifier s'il y a une session active
	hasActiveSession := false
	var count int
	err = a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND status = 'active'`,
		u.ID,
	).Scan(&count)
	if err == nil && count > 0 {
		hasActiveSession = true
	}

	data := struct {
		Title            string
		CurrentUser      *CurrentUser
		Sessions         []SavedSession
		Saved            bool
		Deleted          bool
		HasActiveSession bool
	}{
		Title:            "Mes sessions sauvegardées",
		CurrentUser:      u,
		Sessions:         sessions,
		Saved:            r.URL.Query().Get("saved") == "1",
		Deleted:          r.URL.Query().Get("deleted") == "1",
		HasActiveSession: hasActiveSession,
	}

	if err := a.templates.ExecuteTemplate(w, "sessions.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
