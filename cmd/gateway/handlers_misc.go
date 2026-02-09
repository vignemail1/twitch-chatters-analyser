package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleIndex affiche la page d'accueil
func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	u := currentUser(r.Context())

	// Vérifier s'il y a une session active
	hasActiveSession := false
	if u != nil {
		var count int
		err := a.db.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND status = 'active'`,
			u.ID,
		).Scan(&count)
		if err == nil && count > 0 {
			hasActiveSession = true
		}
	}

	data := struct {
		Title            string
		CurrentUser      *CurrentUser
		HasActiveSession bool
	}{
		Title:            "Twitch Chatters Analyser - Gateway",
		CurrentUser:      u,
		HasActiveSession: hasActiveSession,
	}

	if err := a.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleHealth vérifie l'état de santé de l'application
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Vérifie aussi la DB
	if err := a.db.PingContext(r.Context()); err != nil {
		log.Printf("healthz db error: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleChannels affiche les chaînes modérées par l'utilisateur
func (a *App) handleChannels(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Récupérer la session pour avoir l'access token
	c, err := r.Cookie("tca_session")
	if err != nil || c.Value == "" {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	sess, err := a.getSessionData(r.Context(), c.Value)
	if err != nil {
		log.Printf("getSessionData error: %v", err)
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	channels, err := a.fetchModeratedChannels(r.Context(), sess.AccessToken, u.TwitchUserID)
	if err != nil {
		log.Printf("fetchModeratedChannels error: %v", err)
		http.Error(w, "failed to load channels", http.StatusBadGateway)
		return
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
		Channels         []struct {
			BroadcasterID    string
			BroadcasterLogin string
			BroadcasterName  string
		}
		CaptureEnqueued  bool
		SessionPurged    bool
		HasActiveSession bool
	}{
		Title:            "Mes chaînes modérées",
		CurrentUser:      u,
		Channels:         channels,
		CaptureEnqueued:  r.URL.Query().Get("capture_enqueued") == "1",
		SessionPurged:    r.URL.Query().Get("purged") == "1",
		HasActiveSession: hasActiveSession,
	}

	if err := a.templates.ExecuteTemplate(w, "channels.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleAccountHistory affiche l'historique de changements d'un compte
func (a *App) handleAccountHistory(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Extraire le twitch_user_id depuis l'URL : /accounts/{id}/history
	path := strings.TrimPrefix(r.URL.Path, "/accounts/")
	path = strings.TrimSuffix(path, "/history")
	twitchUserID := path

	if twitchUserID == "" {
		http.Error(w, "missing twitch_user_id", http.StatusBadRequest)
		return
	}

	// Récupérer les infos actuelles du compte
	var currentLogin, currentDisplayName string
	var accountCreatedAt time.Time
	err := a.db.QueryRowContext(r.Context(),
		`SELECT login, display_name, created_at FROM accounts WHERE twitch_user_id = ? LIMIT 1`,
		twitchUserID,
	).Scan(&currentLogin, &currentDisplayName, &accountCreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		log.Printf("query account error: %v", err)
		http.Error(w, "failed to load account", http.StatusInternalServerError)
		return
	}

	// Récupérer l'historique des changements
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT changed_at, old_login, new_login, old_display_name, new_display_name 
         FROM account_history 
         WHERE twitch_user_id = ? 
         ORDER BY changed_at DESC`,
		twitchUserID,
	)
	if err != nil {
		log.Printf("query history error: %v", err)
		http.Error(w, "failed to load history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var history []AccountHistoryChange
	for rows.Next() {
		var h AccountHistoryChange
		if err := rows.Scan(&h.ChangedAt, &h.OldLogin, &h.NewLogin, &h.OldDisplayName, &h.NewDisplayName); err != nil {
			log.Printf("scan history error: %v", err)
			continue
		}
		history = append(history, h)
	}

	data := struct {
		Title              string
		CurrentUser        *CurrentUser
		TwitchUserID       string
		CurrentLogin       string
		CurrentDisplayName string
		AccountCreatedAt   time.Time
		History            []AccountHistoryChange
	}{
		Title:              "Historique des changements de noms",
		CurrentUser:        u,
		TwitchUserID:       twitchUserID,
		CurrentLogin:       currentLogin,
		CurrentDisplayName: currentDisplayName,
		AccountCreatedAt:   accountCreatedAt,
		History:            history,
	}

	if err := a.templates.ExecuteTemplate(w, "account_history.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
