package main

import (
	"context"

	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"io"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type App struct {
	addr      string
	db        *sql.DB
	templates *template.Template

	twitchClientID     string
	twitchClientSecret string
	twitchRedirectURL  string

	analysisBaseURL string
}

type twitchTokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

type twitchUsersResponse struct {
	Data []struct {
		ID              string `json:"id"`
		Login           string `json:"login"`
		DisplayName     string `json:"display_name"`
		ProfileImageURL string `json:"profile_image_url"`
	} `json:"data"`
}

type twitchUser struct {
	ID              string
	Login           string
	DisplayName     string
	ProfileImageURL string
}

type CurrentUser struct {
	ID           int64
	TwitchUserID string
	Login        string
	DisplayName  string
}

type contextKey string

const ctxKeyUser contextKey = "currentUser"

type SessionData struct {
	SessionID   string
	UserID      int64
	AccessToken string
}

type twitchModeratedChannelsResponse struct {
	Data []struct {
		BroadcasterID    string `json:"broadcaster_id"`
		BroadcasterLogin string `json:"broadcaster_login"`
		BroadcasterName  string `json:"broadcaster_name"`
	} `json:"data"`
}

type AnalysisSummary struct {
	SessionUUID   string `json:"session_uuid"`
	TotalAccounts int64  `json:"total_accounts"`
	TopDays       []struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	} `json:"top_days"`
	GeneratedAt time.Time `json:"generated_at"`
}

type SavedSession struct {
	ID          int64
	SessionUUID string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func main() {
	port := getenv("APP_PORT", "8080")

	// DSN MySQL
	dbUser := getenv("DB_USER", "twitch")
	dbPass := getenv("DB_PASSWORD", "twitchpass")
	dbHost := getenv("DB_HOST", "db")
	dbPort := getenv("DB_PORT", "3306")
	dbName := getenv("DB_NAME", "twitch_chatters")

	dsn := dbUser + ":" + dbPass + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

	twitchClientID := getenv("TWITCH_CLIENT_ID", "")
	twitchClientSecret := getenv("TWITCH_CLIENT_SECRET", "")
	twitchRedirectURL := getenv("TWITCH_REDIRECT_URL", "")

	if twitchClientID == "" || twitchClientSecret == "" || twitchRedirectURL == "" {
		log.Println("warning: TWITCH_CLIENT_ID/SECRET/REDIRECT_URL not fully set; auth will not work correctly")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("cannot open DB: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("cannot ping DB: %v", err)
	}

	// Ajouter les fonctions personnalisées pour les templates
	funcMap := template.FuncMap{
		"add": func(a, b int64) int64 { return a + b },
		"mul": func(a, b int64) int64 { return a * b },
		"div": func(a, b int64) int64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
	}

	tmpls, err := template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		log.Fatalf("cannot load templates: %v", err)
	}

	analysisBaseURL := getenv("ANALYSIS_BASE_URL", "http://analysis:8083")

	app := &App{
		addr:               ":" + port,
		db:                 db,
		templates:          tmpls,
		twitchClientID:     twitchClientID,
		twitchClientSecret: twitchClientSecret,
		twitchRedirectURL:  twitchRedirectURL,
		analysisBaseURL:    analysisBaseURL,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/analysis", app.handleAnalysis)
	mux.HandleFunc("/sessions", app.handleSessions)
	mux.HandleFunc("/sessions/capture", app.handleCreateCapture)
	mux.HandleFunc("/sessions/save", app.handleSaveSession)
	mux.HandleFunc("/sessions/delete", app.handleDeleteSession)
	mux.HandleFunc("/sessions/purge", app.handlePurgeSession)
	mux.HandleFunc("/channels", app.handleChannels)
	mux.HandleFunc("/auth/login", app.handleAuthLogin)
	mux.HandleFunc("/auth/callback", app.handleAuthCallback)
	mux.HandleFunc("/auth/logout", app.handleLogout)
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/", app.handleIndex)

	fileServer := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	handler := loggingMiddleware(app.loadCurrentUser(mux))

	httpServer := &http.Server{
		Addr:              app.addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("gateway listening on %s", app.addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	// vérifie aussi la DB
	if err := a.db.PingContext(r.Context()); err != nil {
		log.Printf("healthz db error: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s from %s in %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

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

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

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

	// Redirection vers la home (plus tard, vers /channels ou /sessions)
	http.Redirect(w, r, "/", http.StatusFound)

}

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

func (a *App) revokeTwitchToken(ctx context.Context, accessToken string) error {
	data := url.Values{}
	data.Set("client_id", a.twitchClientID)
	data.Set("token", accessToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/revoke", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twitch revoke returned %s", resp.Status)
	}
	return nil
}

func (a *App) exchangeCodeForToken(ctx context.Context, code string) (*twitchTokenResponse, error) {
	if a.twitchClientID == "" || a.twitchClientSecret == "" || a.twitchRedirectURL == "" {
		return nil, fmt.Errorf("twitch client not configured")
	}

	data := url.Values{}
	data.Set("client_id", a.twitchClientID)
	data.Set("client_secret", a.twitchClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", a.twitchRedirectURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch token endpoint returned %s", resp.Status)
	}

	var tr twitchTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

func (a *App) fetchTwitchUser(ctx context.Context, accessToken string) (*twitchUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch users endpoint returned %s", resp.Status)
	}

	var ur twitchUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		return nil, err
	}
	if len(ur.Data) == 0 {
		return nil, fmt.Errorf("no user data in response")
	}
	d := ur.Data[0]
	return &twitchUser{
		ID:              d.ID,
		Login:           d.Login,
		DisplayName:     d.DisplayName,
		ProfileImageURL: d.ProfileImageURL,
	}, nil
}

func (a *App) upsertUser(ctx context.Context, u twitchUser) (int64, error) {
	// upsert based on twitch_user_id
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

func currentUser(ctx context.Context) *CurrentUser {
	u, _ := ctx.Value(ctxKeyUser).(*CurrentUser)
	return u
}

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

func (a *App) fetchModeratedChannels(ctx context.Context, accessToken, userID string) ([]struct {
	BroadcasterID    string
	BroadcasterLogin string
	BroadcasterName  string
}, error) {
	params := url.Values{}
	params.Set("user_id", userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitch.tv/helix/moderation/channels?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", a.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("forbidden: missing scope user:read:moderated_channels or moderator rights")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch moderation/channels returned %s", resp.Status)
	}

	var tr twitchModeratedChannelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}

	out := make([]struct {
		BroadcasterID    string
		BroadcasterLogin string
		BroadcasterName  string
	}, 0, len(tr.Data))

	for _, c := range tr.Data {
		out = append(out, struct {
			BroadcasterID    string
			BroadcasterLogin string
			BroadcasterName  string
		}{
			BroadcasterID:    c.BroadcasterID,
			BroadcasterLogin: c.BroadcasterLogin,
			BroadcasterName:  c.BroadcasterName,
		})
	}

	return out, nil
}

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

	// Pour l'instant, simple redirection avec un paramètre de succès
	http.Redirect(w, r, "/channels?capture_enqueued=1", http.StatusFound)
}

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
			// Pas de session active, rediriger vers /channels avec message
			http.Redirect(w, r, "/channels?purge_no_session=1", http.StatusFound)
			return
		}
		log.Printf("query session error: %v", err)
		http.Error(w, "failed to query session", http.StatusInternalServerError)
		return
	}

	// Suppression en cascade : capture_chatters -> captures -> session
	// 1. Détruire capture_chatters
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

	// 2. Détruire captures
	_, err = a.db.ExecContext(r.Context(), `DELETE FROM captures WHERE session_id = ?`, sessionID)
	if err != nil {
		log.Printf("delete captures error: %v", err)
		http.Error(w, "failed to purge session", http.StatusInternalServerError)
		return
	}

	// 3. Marquer la session comme 'deleted' (valeur valide dans l'ENUM)
	_, err = a.db.ExecContext(r.Context(), `UPDATE sessions SET status = 'deleted', updated_at = NOW(6) WHERE id = ?`, sessionID)
	if err != nil {
		log.Printf("update session error: %v", err)
		http.Error(w, "failed to purge session", http.StatusInternalServerError)
		return
	}

	log.Printf("session %d purged by user %d", sessionID, u.ID)
	// Rediriger vers /channels avec message de succès au lieu de /analysis
	http.Redirect(w, r, "/channels?purged=1", http.StatusFound)
}

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

func (a *App) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Récupérer la session d'analyse active
	sessionUUID, err := a.getActiveSessionUUID(r.Context(), u.ID)
	if err != nil {
		log.Printf("getActiveSessionUUID error: %v", err)
		http.Error(w, "no active analysis session", http.StatusNotFound)
		return
	}

	// Optionnel : filtre broadcaster_id en query string
	broadcasterID := r.URL.Query().Get("broadcaster_id")

	summary, err := a.fetchAnalysisSummary(r.Context(), sessionUUID, broadcasterID)
	if err != nil {
		log.Printf("fetchAnalysisSummary error: %v", err)
		http.Error(w, "failed to load analysis", http.StatusBadGateway)
		return
	}

	data := struct {
		Title         string
		CurrentUser   *CurrentUser
		SessionUUID   string
		Summary       *AnalysisSummary
		BroadcasterID string
		Purged        bool
	}{
		Title:         "Analyse de session",
		CurrentUser:   u,
		SessionUUID:   sessionUUID,
		Summary:       summary,
		BroadcasterID: broadcasterID,
		Purged:        r.URL.Query().Get("purged") == "1",
	}

	if err := a.templates.ExecuteTemplate(w, "analysis_page", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}

}

func (a *App) fetchAnalysisSummary(ctx context.Context, sessionUUID, broadcasterID string) (*AnalysisSummary, error) {
	urlStr := a.analysisBaseURL + "/sessions/" + sessionUUID + "/summary"
	if broadcasterID != "" {
		q := url.Values{}
		q.Set("broadcaster_id", broadcasterID)
		urlStr += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("analysis returned %s: %s", resp.Status, string(body))
	}

	var s AnalysisSummary
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}
