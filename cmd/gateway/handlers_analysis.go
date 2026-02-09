package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// handleAnalysis affiche l'analyse de la session active
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
	broadcasterIDs := r.URL.Query()["broadcaster_id"]

	summary, err := a.fetchAnalysisSummary(r.Context(), sessionUUID, strings.Join(broadcasterIDs, ","))
	if err != nil {
		log.Printf("fetchAnalysisSummary error: %v", err)
		http.Error(w, "failed to load analysis", http.StatusBadGateway)
		return
	}

	data := struct {
		Title          string
		CurrentUser    *CurrentUser
		SessionUUID    string
		Summary        *AnalysisSummary
		BroadcasterIDs []string
		Purged         bool
		IsSaved        bool
	}{
		Title:          "Analyse de session",
		CurrentUser:    u,
		SessionUUID:    sessionUUID,
		Summary:        summary,
		BroadcasterIDs: broadcasterIDs,
		Purged:         r.URL.Query().Get("purged") == "1",
		IsSaved:        false,
	}

	if err := a.templates.ExecuteTemplate(w, "analysis_page", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleSavedAnalysis affiche l'analyse d'une session sauvegardée
func (a *App) handleSavedAnalysis(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Extraire le session UUID depuis l'URL : /analysis/saved/{uuid}
	path := strings.TrimPrefix(r.URL.Path, "/analysis/saved/")
	sessionUUID := strings.TrimSuffix(path, "/")

	if sessionUUID == "" {
		http.Error(w, "missing session UUID", http.StatusBadRequest)
		return
	}

	// Vérifier que la session existe et appartient à l'utilisateur
	var status string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT status FROM sessions WHERE user_id = ? AND session_uuid = ? AND status = 'saved' LIMIT 1`,
		u.ID, sessionUUID,
	).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "session not found or not saved", http.StatusNotFound)
			return
		}
		log.Printf("query session error: %v", err)
		http.Error(w, "failed to query session", http.StatusInternalServerError)
		return
	}

	// Optionnel : filtre broadcaster_id en query string
	broadcasterIDs := r.URL.Query()["broadcaster_id"]

	summary, err := a.fetchAnalysisSummary(r.Context(), sessionUUID, strings.Join(broadcasterIDs, ","))
	if err != nil {
		log.Printf("fetchAnalysisSummary error: %v", err)
		http.Error(w, "failed to load analysis", http.StatusBadGateway)
		return
	}

	data := struct {
		Title          string
		CurrentUser    *CurrentUser
		SessionUUID    string
		Summary        *AnalysisSummary
		BroadcasterIDs []string
		Purged         bool
		IsSaved        bool
	}{
		Title:          "Analyse de session sauvegardée",
		CurrentUser:    u,
		SessionUUID:    sessionUUID,
		Summary:        summary,
		BroadcasterIDs: broadcasterIDs,
		Purged:         false,
		IsSaved:        true,
	}

	if err := a.templates.ExecuteTemplate(w, "analysis_page", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// fetchAnalysisSummary récupère le résumé d'analyse depuis le service analysis
func (a *App) fetchAnalysisSummary(ctx context.Context, sessionUUID, broadcasterIDs string) (*AnalysisSummary, error) {
	urlStr := a.analysisBaseURL + "/sessions/" + sessionUUID + "/summary"
	if broadcasterIDs != "" {
		q := url.Values{}
		q.Set("broadcaster_id", broadcasterIDs)
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

// handleAnalysisExport exporte la session active en CSV ou JSON
func (a *App) handleAnalysisExport(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Récupérer la session active
	sessionUUID, err := a.getActiveSessionUUID(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "no active session", http.StatusNotFound)
		return
	}

	format := r.URL.Query().Get("format")
	if format != "csv" && format != "json" {
		format = "json" // par défaut
	}

	a.exportSession(w, r, sessionUUID, format)
}

// handleSessionExport exporte une session sauvegardée en CSV ou JSON
func (a *App) handleSessionExport(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Extraire le session UUID depuis l'URL : /sessions/export/{uuid}
	path := strings.TrimPrefix(r.URL.Path, "/sessions/export/")
	sessionUUID := strings.TrimSuffix(path, "/")

	if sessionUUID == "" {
		http.Error(w, "missing session UUID", http.StatusBadRequest)
		return
	}

	// Vérifier que la session appartient à l'utilisateur
	var status string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT status FROM sessions WHERE user_id = ? AND session_uuid = ? AND status = 'saved' LIMIT 1`,
		u.ID, sessionUUID,
	).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		log.Printf("query session error: %v", err)
		http.Error(w, "failed to query session", http.StatusInternalServerError)
		return
	}

	format := r.URL.Query().Get("format")
	if format != "csv" && format != "json" {
		format = "json" // par défaut
	}

	a.exportSession(w, r, sessionUUID, format)
}

// exportSession exporte les données d'une session en CSV ou JSON
func (a *App) exportSession(w http.ResponseWriter, r *http.Request, sessionUUID, format string) {
	// Récupérer les données de la session
	var sessionID int64
	err := a.db.QueryRowContext(r.Context(),
		`SELECT id FROM sessions WHERE session_uuid = ? LIMIT 1`,
		sessionUUID,
	).Scan(&sessionID)
	if err != nil {
		log.Printf("query session error: %v", err)
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Récupérer tous les accounts capturés
	query := `
SELECT 
    a.twitch_user_id,
    a.login,
    a.display_name,
    a.created_at,
    COUNT(DISTINCT cc.capture_id) as seen_count,
    MIN(c.captured_at) as first_seen,
    MAX(c.captured_at) as last_seen
FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
JOIN accounts a ON cc.account_id = a.id
WHERE c.session_id = ?
GROUP BY a.id, a.twitch_user_id, a.login, a.display_name, a.created_at
ORDER BY seen_count DESC, a.login ASC
`

	rows, err := a.db.QueryContext(r.Context(), query, sessionID)
	if err != nil {
		log.Printf("query accounts error: %v", err)
		http.Error(w, "failed to load accounts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var accounts []ExportAccountData
	for rows.Next() {
		var acc ExportAccountData
		if err := rows.Scan(&acc.TwitchUserID, &acc.Login, &acc.DisplayName, &acc.CreatedAt, &acc.SeenCount, &acc.FirstSeen, &acc.LastSeen); err != nil {
			log.Printf("scan account error: %v", err)
			continue
		}
		accounts = append(accounts, acc)
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session_%s.csv", sessionUUID))

		csvWriter := csv.NewWriter(w)
		defer csvWriter.Flush()

		// Header
		_ = csvWriter.Write([]string{"twitch_user_id", "login", "display_name", "created_at", "seen_count", "first_seen", "last_seen"})

		// Rows
		for _, acc := range accounts {
			_ = csvWriter.Write([]string{
				acc.TwitchUserID,
				acc.Login,
				acc.DisplayName,
				acc.CreatedAt.Format(time.RFC3339),
				fmt.Sprintf("%d", acc.SeenCount),
				acc.FirstSeen.Format(time.RFC3339),
				acc.LastSeen.Format(time.RFC3339),
			})
		}
	} else {
		// JSON
		exportData := ExportData{
			SessionUUID: sessionUUID,
			ExportedAt:  time.Now().UTC(),
			Accounts:    accounts,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session_%s.json", sessionUUID))

		if err := json.NewEncoder(w).Encode(exportData); err != nil {
			log.Printf("json encode error: %v", err)
		}
	}
}
