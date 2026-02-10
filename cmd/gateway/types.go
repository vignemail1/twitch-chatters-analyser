package main

import (
	"context"
	"database/sql"
	"html/template"
	"net/http"
	"time"
)

// App contient la configuration et les dépendances de l'application
type App struct {
	addr      string
	db        *sql.DB
	templates *template.Template

	twitchClientID     string
	twitchClientSecret string
	twitchRedirectURL  string

	analysisBaseURL string
}

// CurrentUser représente l'utilisateur actuellement connecté
type CurrentUser struct {
	ID           int64
	TwitchUserID string
	Login        string
	DisplayName  string
}

// SessionData contient les données d'une session web
type SessionData struct {
	SessionID   string
	UserID      int64
	AccessToken string
}

// SavedSession représente une session d'analyse sauvegardée
type SavedSession struct {
	ID          int64
	SessionUUID string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExportData structure pour l'export de données
type ExportData struct {
	SessionUUID string              `json:"session_uuid"`
	ExportedAt  time.Time           `json:"exported_at"`
	Accounts    []ExportAccountData `json:"accounts"`
}

// ExportAccountData données d'un compte pour l'export
type ExportAccountData struct {
	TwitchUserID string    `json:"twitch_user_id"`
	Login        string    `json:"login"`
	DisplayName  string    `json:"display_name"`
	CreatedAt    time.Time `json:"created_at"`
	SeenCount    int64     `json:"seen_count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// AccountHistoryChange représente un changement de nom de compte
type AccountHistoryChange struct {
	ChangedAt      time.Time
	OldLogin       string
	NewLogin       string
	OldDisplayName string
	NewDisplayName string
}

// Broadcaster représente un broadcaster Twitch
type Broadcaster struct {
	BroadcasterID    string `json:"broadcaster_id"`
	BroadcasterLogin string `json:"broadcaster_login"`
	CaptureCount     int64  `json:"capture_count"`
}

// SuspiciousAccount représente un compte suspect
type SuspiciousAccount struct {
	TwitchUserID string `json:"twitch_user_id"`
	Login        string `json:"login"`
	DisplayName  string `json:"display_name"`
	RenameCount  int64  `json:"rename_count"`
}

// AnalysisSummary résumé d'une analyse de session
type AnalysisSummary struct {
	SessionUUID            string              `json:"session_uuid"`
	TotalAccounts          int64               `json:"total_accounts"`
	TopDays                []struct {
		Date   string   `json:"date"`
		Count  int64    `json:"count"`
		Logins []string `json:"logins"`
	} `json:"top_days"`
	Broadcasters           []Broadcaster       `json:"broadcasters"`
	SuspiciousRenamesCount int64               `json:"suspicious_renames_count"`
	SuspiciousAccounts     []SuspiciousAccount `json:"suspicious_accounts"`
	GeneratedAt            time.Time           `json:"generated_at"`
}

// twitchUser représente un utilisateur Twitch
type twitchUser struct {
	ID              string
	Login           string
	DisplayName     string
	ProfileImageURL string
}

// twitchTokenResponse réponse OAuth de Twitch
type twitchTokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

// twitchUsersResponse réponse de l'API users de Twitch
type twitchUsersResponse struct {
	Data []struct {
		ID              string `json:"id"`
		Login           string `json:"login"`
		DisplayName     string `json:"display_name"`
		ProfileImageURL string `json:"profile_image_url"`
	} `json:"data"`
}

// twitchModeratedChannelsResponse réponse de l'API moderation/channels
type twitchModeratedChannelsResponse struct {
	Data []struct {
		BroadcasterID    string `json:"broadcaster_id"`
		BroadcasterLogin string `json:"broadcaster_login"`
		BroadcasterName  string `json:"broadcaster_name"`
	} `json:"data"`
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// contextKey type pour les clés de contexte
type contextKey string

const ctxKeyUser contextKey = "currentUser"

// currentUser récupère l'utilisateur depuis le contexte
func currentUser(ctx context.Context) *CurrentUser {
	u, _ := ctx.Value(ctxKeyUser).(*CurrentUser)
	return u
}
