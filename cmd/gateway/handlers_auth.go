package main

import (
	"log"
	"net/http"
	"net/url"
)

// handleAuthLogin initie le flux OAuth avec Twitch
func (a *App) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if a.twitchClientID == "" || a.twitchRedirectURL == "" {
		http.Error(w, "Twitch auth not configured", http.StatusInternalServerError)
		return
	}

	state, err := randomHex(32)
	if err != nil {
		log.Printf("cannot generate state: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "tca_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // à passer à true en prod si HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 min
	})

	params := url.Values{}
	params.Set("client_id", a.twitchClientID)
	params.Set("redirect_uri", a.twitchRedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", "user:read:moderated_channels moderator:read:chatters")
	params.Set("state", state)

	authURL := "https://id.twitch.tv/oauth2/authorize?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleAuthCallback gère le callback OAuth de Twitch
func (a *App) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	// Vérification de state
	q := r.URL.Query()
	state := q.Get("state")
	code := q.Get("code")
	errorParam := q.Get("error")

	if errorParam != "" {
		log.Printf("twitch auth error: %s", errorParam)
		http.Error(w, "Twitch auth error", http.StatusBadRequest)
		return
	}

	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	stateCookie, err := r.Cookie("tca_oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != state {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Échange code -> token
	token, err := a.exchangeCodeForToken(r.Context(), code)
	if err != nil {
		log.Printf("exchange code error: %v", err)
		http.Error(w, "failed to authenticate with Twitch", http.StatusBadGateway)
		return
	}

	// Récupérer les infos de l'utilisateur Twitch
	tUser, err := a.fetchTwitchUser(r.Context(), token.AccessToken)
	if err != nil {
		log.Printf("fetch twitch user error: %v", err)
		http.Error(w, "failed to fetch Twitch user", http.StatusBadGateway)
		return
	}

	// Upsert de l'utilisateur en DB
	userID, err := a.upsertUser(r.Context(), *tUser)
	if err != nil {
		log.Printf("upsert user error: %v", err)
		http.Error(w, "failed to store user", http.StatusInternalServerError)
		return
	}

	// Créer une web_session
	sessionID, err := a.createWebSession(r.Context(), userID, token.AccessToken, token.RefreshToken, token.Scope)
	if err != nil {
		log.Printf("create web session error: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Poser le cookie de session applicatif
	a.setSessionCookie(w, sessionID)

	log.Printf("authenticated Twitch user: id=%s login=%s local_user_id=%d", tUser.ID, tUser.Login, userID)

	// Redirection vers la home
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleLogout déconnecte l'utilisateur et purge sa session active
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Récupérer le token pour révoquer
	c, err := r.Cookie("tca_session")
	var accessToken string
	if err == nil && c.Value != "" {
		sess, err := a.getSessionData(r.Context(), c.Value)
		if err == nil {
			accessToken = sess.AccessToken
		}
	}

	// Purger la session active si elle existe et n'est pas saved
	var sessionID int64
	var status string
	err = a.db.QueryRowContext(r.Context(),
		`SELECT id, status FROM sessions WHERE user_id = ? AND status = 'active' LIMIT 1`,
		u.ID,
	).Scan(&sessionID, &status)
	if err == nil {
		// Supprimer les captures
		_, _ = a.db.ExecContext(r.Context(), `
DELETE cc FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
WHERE c.session_id = ?
`, sessionID)
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM captures WHERE session_id = ?`, sessionID)
		_, _ = a.db.ExecContext(r.Context(), `UPDATE sessions SET status = 'deleted', updated_at = NOW(6) WHERE id = ?`, sessionID)
		log.Printf("session %d auto-purged on logout by user %d", sessionID, u.ID)
	}

	// Supprimer la web_session
	if c != nil && c.Value != "" {
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM web_sessions WHERE session_id = ?`, c.Value)
	}

	// Révoquer le token Twitch
	if accessToken != "" {
		_ = a.revokeTwitchToken(r.Context(), accessToken)
	}

	// Supprimer les cookies
	http.SetCookie(w, &http.Cookie{
		Name:     "tca_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.SetCookie(w, &http.Cookie{
		Name:   "tca_oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	log.Printf("user %d logged out", u.ID)
	http.Redirect(w, r, "/?logged_out=1", http.StatusFound)
}
