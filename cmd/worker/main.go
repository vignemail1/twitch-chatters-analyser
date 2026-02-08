package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Job struct {
	ID      int64
	Type    string
	Payload json.RawMessage
}

type FetchChattersPayload struct {
	SessionID        int64  `json:"session_id"`
	TwitchUserID     string `json:"twitch_user_id"`
	BroadcasterID    string `json:"broadcaster_id"`
	BroadcasterLogin string `json:"broadcaster_login"`
}

type helixChattersResponse struct {
	Data []struct {
		UserID    string `json:"user_id"`
		UserLogin string `json:"user_login"`
		UserName  string `json:"user_name"`
	} `json:"data"`
	Pagination struct {
		Cursor string `json:"cursor"`
	} `json:"pagination"`
}

type FetchUsersInfoPayload struct {
	SessionID int64    `json:"session_id"`
	UserIDs   []string `json:"user_ids"`
}
type helixUser struct {
	ID             string `json:"id"`
	Login          string `json:"login"`
	DisplayName    string `json:"display_name"`
	Type           string `json:"type"`
	BroadcasterTyp string `json:"broadcaster_type"`
	ViewCount      int    `json:"view_count"`
	CreatedAt      string `json:"created_at"`
}

type helixUsersResponse struct {
	Data []helixUser `json:"data"`
}

func main() {
	dbUser := getenv("DB_USER", "twitch")
	dbPass := getenv("DB_PASSWORD", "twitchpass")
	dbHost := getenv("DB_HOST", "db")
	dbPort := getenv("DB_PORT", "3306")
	dbName := getenv("DB_NAME", "twitch_chatters")

	dsn := dbUser + ":" + dbPass + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("cannot open DB: %v", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("cannot ping DB: %v", err)
	}

	pollIntervalSecs := getenvInt("JOB_POLL_INTERVAL", 2)

	twitchAPIBase := getenv("TWITCH_API_BASE_URL", "http://twitch-api:8081")
	log.Printf("worker started, poll interval=%ds, twitch-api=%s", pollIntervalSecs, twitchAPIBase)

	ticker := time.NewTicker(time.Duration(pollIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := processOneJob(db); err != nil {
				log.Printf("processOneJob error: %v", err)
			}
		}
	}
}

func processOneJob(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// MySQL 8+: FOR UPDATE SKIP LOCKED pour éviter les conflits entre workers
	row := tx.QueryRowContext(ctx, `
SELECT id, type, payload
FROM jobs
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT 1
FOR UPDATE
`)
	var job Job
	if err := row.Scan(&job.ID, &job.Type, &job.Payload); err != nil {
		if err == sql.ErrNoRows {
			// aucun job en attente
			return nil
		}
		return err
	}

	// marquer le job en running
	if _, err := tx.ExecContext(ctx, `
UPDATE jobs
SET status = 'running', started_at = NOW(6)
WHERE id = ?`, job.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// traiter le job hors transaction
	log.Printf("picked job id=%d type=%s payload=%s", job.ID, job.Type, string(job.Payload))

	var errJob error
	switch job.Type {
	case "FETCH_CHATTERS":
		errJob = handleFetchChatters(ctx, db, job)
	case "FETCH_USERS_INFO":
		errJob = handleFetchUsersInfo(ctx, db, job)
	default:
		log.Printf("unknown job type %s, marking as failed", job.Type)
		errJob = fmt.Errorf("unknown job type")
	}

	if errJob != nil {
		log.Printf("job %d error: %v", job.ID, errJob)
		if err := markJobDone(db, job.ID, errJob.Error()); err != nil {
			log.Printf("markJobDone error for job %d: %v", job.ID, err)
		}
		return nil
	}

	if err := markJobDone(db, job.ID, ""); err != nil {
		log.Printf("markJobDone error for job %d: %v", job.ID, err)
	}
	return nil

}

func markJobDone(db *sql.DB, jobID int64, errorMsg string) error {
	status := "done"
	if errorMsg != "" {
		status = "failed"
	}
	_, err := db.Exec(`
UPDATE jobs
SET status = ?, finished_at = NOW(6), error_message = ?
WHERE id = ?`, status, errorMsg, jobID)
	return err
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func handleFetchChatters(ctx context.Context, db *sql.DB, job Job) error {
	var payload FetchChattersPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	// Récupérer le token Twitch de l'utilisateur (via web_sessions)
	var accessToken string
	err := db.QueryRowContext(ctx,
		`SELECT ws.access_token
         FROM web_sessions ws
         JOIN sessions s ON s.user_id = ws.user_id
         WHERE s.id = ? AND ws.expires_at > NOW(6)
         ORDER BY ws.last_activity_at DESC
         LIMIT 1`,
		payload.SessionID,
	).Scan(&accessToken)
	if err != nil {
		return fmt.Errorf("cannot get access token for session %d: %w", payload.SessionID, err)
	}

	// Appeler le service twitch-api proxy pour /chatters
	chatters, err := fetchAllChatters(ctx, accessToken, payload.BroadcasterID, payload.TwitchUserID)
	if err != nil {
		return fmt.Errorf("fetchAllChatters: %w", err)
	}

	log.Printf("[FETCH_CHATTERS] session_id=%d broadcaster=%s login=%s chatters_count=%d",
		payload.SessionID, payload.BroadcasterID, payload.BroadcasterLogin, len(chatters))

	// Enregistrer la capture + les chatters
	if err := storeCapture(ctx, db, payload, chatters); err != nil {
		return fmt.Errorf("storeCapture: %w", err)
	}

	return nil
}

func fetchAllChatters(ctx context.Context, accessToken, broadcasterID, moderatorID string) ([]string, error) {
	twitchAPIBase := getenv("TWITCH_API_BASE_URL", "http://twitch-api:8081")

	allIDs := make([]string, 0, 1024)
	cursor := ""
	const pageSize = 1000 // max per page

	for {
		params := url.Values{}
		params.Set("broadcaster_id", broadcasterID)
		params.Set("moderator_id", moderatorID)
		params.Set("first", strconv.Itoa(pageSize))
		if cursor != "" {
			params.Set("after", cursor)
		}

		// Appel au proxy twitch-api au lieu de l'API Twitch directement
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, twitchAPIBase+"/chatters?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		func() {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				log.Printf("rate limited from twitch-api proxy, sleeping 5s")
				time.Sleep(5 * time.Second)
				return
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				err = fmt.Errorf("twitch-api /chatters returned %s: %s", resp.Status, string(body))
				return
			}

			var hr helixChattersResponse
			if e := json.NewDecoder(resp.Body).Decode(&hr); e != nil {
				err = e
				return
			}

			for _, c := range hr.Data {
				allIDs = append(allIDs, c.UserID)
			}
			cursor = hr.Pagination.Cursor
		}()
		if err != nil {
			return nil, err
		}

		if cursor == "" {
			break
		}

		// Léger sleep pour éviter de spammer le proxy
		time.Sleep(200 * time.Millisecond)
	}

	return allIDs, nil
}

func storeCapture(ctx context.Context, db *sql.DB, payload FetchChattersPayload, chatters []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()

	// Créer la capture
	res, err := tx.ExecContext(ctx, `
INSERT INTO captures (session_id, broadcaster_id, broadcaster_login, captured_at, chatters_count, new_users_count)
VALUES (?, ?, ?, ?, ?, 0)
`, payload.SessionID, payload.BroadcasterID, payload.BroadcasterLogin, now, len(chatters))
	if err != nil {
		return err
	}
	captureID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	// Insérer les chatters (twitch_user_id) liés à la capture
	if len(chatters) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO capture_chatters (capture_id, twitch_user_id)
VALUES (?, ?)
`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, uid := range chatters {
			if _, err := stmt.ExecContext(ctx, captureID, uid); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("[STORE_CAPTURE] capture_id=%d session_id=%d chatters=%d", captureID, payload.SessionID, len(chatters))
	// Créer un job FETCH_USERS_INFO pour enrichir les comptes
	if len(chatters) > 0 {
		payload := map[string]interface{}{
			"session_id": payload.SessionID,
			"user_ids":   chatters,
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx,
			`INSERT INTO jobs (type, payload, status, created_at) VALUES ('FETCH_USERS_INFO', ?, 'pending', NOW(6))`,
			string(payloadJSON),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func handleFetchUsersInfo(ctx context.Context, db *sql.DB, job Job) error {
	var payload FetchUsersInfoPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	if len(payload.UserIDs) == 0 {
		log.Printf("[FETCH_USERS_INFO] job %d: no user_ids", job.ID)
		return nil
	}

	// On déduplique pour éviter de faire des requêtes inutiles
	unique := make(map[string]struct{}, len(payload.UserIDs))
	for _, id := range payload.UserIDs {
		unique[id] = struct{}{}
	}
	userIDs := make([]string, 0, len(unique))
	for id := range unique {
		userIDs = append(userIDs, id)
	}

	// Récupérer un token (on réutilise la même logique que pour FETCH_CHATTERS)
	var accessToken string
	err := db.QueryRowContext(ctx,
		`SELECT ws.access_token
         FROM web_sessions ws
         JOIN sessions s ON s.user_id = ws.user_id
         WHERE s.id = ? AND ws.expires_at > NOW(6)
         ORDER BY ws.last_activity_at DESC
         LIMIT 1`,
		payload.SessionID,
	).Scan(&accessToken)
	if err != nil {
		return fmt.Errorf("cannot get access token for session %d: %w", payload.SessionID, err)
	}

	users, err := fetchUsersInfoFromTwitchAPI(ctx, accessToken, userIDs)
	if err != nil {
		return fmt.Errorf("fetchUsersInfoFromTwitchAPI: %w", err)
	}

	if err := upsertTwitchUsers(ctx, db, users); err != nil {
		return fmt.Errorf("upsertTwitchUsers: %w", err)
	}

	log.Printf("[FETCH_USERS_INFO] job %d session_id=%d users_enriched=%d", job.ID, payload.SessionID, len(users))
	return nil
}

func fetchUsersInfoFromTwitchAPI(ctx context.Context, accessToken string, userIDs []string) ([]helixUser, error) {
	twitchAPIBase := getenv("TWITCH_API_BASE_URL", "http://twitch-api:8081")

	const batchSize = 100 // max IDs par requête
	all := make([]helixUser, 0, len(userIDs))

	for start := 0; start < len(userIDs); start += batchSize {
		end := start + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		batch := userIDs[start:end]

		params := url.Values{}
		for _, id := range batch {
			params.Add("id", id)
		}

		// Appel au proxy twitch-api
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, twitchAPIBase+"/users?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		func() {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				log.Printf("rate limited from twitch-api proxy on /users, sleeping 5s")
				time.Sleep(5 * time.Second)
				return
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				err = fmt.Errorf("twitch-api /users returned %s: %s", resp.Status, string(body))
				return
			}

			var ur helixUsersResponse
			if e := json.NewDecoder(resp.Body).Decode(&ur); e != nil {
				err = e
				return
			}
			all = append(all, ur.Data...)
		}()
		if err != nil {
			return nil, err
		}

		time.Sleep(100 * time.Millisecond)
	}

	return all, nil
}

func upsertTwitchUsers(ctx context.Context, db *sql.DB, users []helixUser) error {
	if len(users) == 0 {
		return nil
	}
	now := time.Now().UTC()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO twitch_users (twitch_user_id, login, display_name, created_at, broadcaster_type, type, view_count, last_fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  login = VALUES(login),
  display_name = VALUES(display_name),
  created_at = VALUES(created_at),
  broadcaster_type = VALUES(broadcaster_type),
  type = VALUES(type),
  view_count = VALUES(view_count),
  last_fetched_at = VALUES(last_fetched_at)
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Track name changes
	for _, u := range users {
		// Check if name changed
		var oldLogin, oldDisplayName sql.NullString
		err := tx.QueryRowContext(ctx,
			`SELECT login, display_name FROM twitch_users WHERE twitch_user_id = ?`,
			u.ID,
		).Scan(&oldLogin, &oldDisplayName)
		
		if err == nil && oldLogin.Valid {
			// User exists, check for changes
			if oldLogin.String != u.Login || oldDisplayName.String != u.DisplayName {
				// Name changed, log it
				_, _ = tx.ExecContext(ctx, `
INSERT INTO twitch_user_names (twitch_user_id, login, display_name, detected_at)
VALUES (?, ?, ?, NOW(6))
`, u.ID, u.Login, u.DisplayName)
				log.Printf("[NAME_CHANGE] user_id=%s old_login=%s new_login=%s old_display=%s new_display=%s",
					u.ID, oldLogin.String, u.Login, oldDisplayName.String, u.DisplayName)
			}
		}

		var createdAt *time.Time
		if u.CreatedAt != "" {
			// created_at est renvoyé en ISO 8601, qu'on peut parser en time.Time
			t, err := time.Parse(time.RFC3339, u.CreatedAt)
			if err == nil {
				createdAt = &t
			}
		}
		var createdAtVal interface{}
		if createdAt != nil {
			createdAtVal = *createdAt
		} else {
			createdAtVal = nil
		}

		if _, err := stmt.ExecContext(ctx,
			u.ID,
			u.Login,
			u.DisplayName,
			createdAtVal,
			u.BroadcasterTyp,
			u.Type,
			u.ViewCount,
			now,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
