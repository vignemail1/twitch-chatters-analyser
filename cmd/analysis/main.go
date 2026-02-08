package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type App struct {
	db   *sql.DB
	addr string
}

type SessionSummary struct {
	SessionUUID   string    `json:"session_uuid"`
	TotalAccounts int64     `json:"total_accounts"`
	TopDays       []TopDay  `json:"top_days"`
	GeneratedAt   time.Time `json:"generated_at"`
}

type TopDay struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int64  `json:"count"`
}

func main() {
	dbUser := getenv("DB_USER", "twitch")
	dbPass := getenv("DB_PASSWORD", "twitchpass")
	dbHost := getenv("DB_HOST", "db")
	dbPort := getenv("DB_PORT", "3306")
	dbName := getenv("DB_NAME", "twitch_chatters")
	port := getenv("APP_PORT", "8083")

	dsn := dbUser + ":" + dbPass + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

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

	app := &App{
		db:   db,
		addr: ":" + port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/sessions/", app.handleSessionSummary)

	srv := &http.Server{
		Addr:              app.addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("analysis service listening on %s", app.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := a.db.PingContext(r.Context()); err != nil {
		log.Printf("health db error: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// GET /sessions/{uuid}/summary
func (a *App) handleSessionSummary(w http.ResponseWriter, r *http.Request) {
	// URL attendue : /sessions/<session_uuid>/summary
	path := r.URL.Path // ex: /sessions/abcd-1234/summary

	const prefix = "/sessions/"
	if !strings.HasPrefix(path, prefix) {
		http.NotFound(w, r)
		return
	}

	rest := strings.TrimPrefix(path, prefix) // ex: "abcd-1234/summary"
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "summary" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	sessionUUID := parts[0]

	// Optionnel : filtre broadcaster_id
	broadcasterID := r.URL.Query().Get("broadcaster_id")

	summary, err := a.buildSessionSummary(r.Context(), sessionUUID, broadcasterID)
	if err != nil {
		log.Printf("buildSessionSummary error: %v", err)
		http.Error(w, "failed to build summary", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func (a *App) buildSessionSummary(ctx context.Context, sessionUUID, broadcasterID string) (*SessionSummary, error) {
	// Récupérer l'id interne de la session
	var sessionID int64
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM sessions WHERE session_uuid = ?`,
		sessionUUID,
	).Scan(&sessionID)
	if err != nil {
		return nil, err
	}

	// Nombre total de comptes distincts pour cette session (et broadcaster si filtré)
	var total int64
	if broadcasterID == "" {
		err = a.db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT cc.twitch_user_id)
FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
WHERE c.session_id = ?
`, sessionID).Scan(&total)
	} else {
		err = a.db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT cc.twitch_user_id)
FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
WHERE c.session_id = ? AND c.broadcaster_id = ?
`, sessionID, broadcasterID).Scan(&total)
	}
	if err != nil {
		return nil, err
	}

	// Top 10 des jours de création
	var rows *sql.Rows
	if broadcasterID == "" {
		rows, err = a.db.QueryContext(ctx, `
SELECT DATE(tu.created_at) AS d, COUNT(*) AS cnt
FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
JOIN twitch_users tu ON tu.twitch_user_id = cc.twitch_user_id
WHERE c.session_id = ?
GROUP BY d
ORDER BY cnt DESC
LIMIT 10
`, sessionID)
	} else {
		rows, err = a.db.QueryContext(ctx, `
SELECT DATE(tu.created_at) AS d, COUNT(*) AS cnt
FROM capture_chatters cc
JOIN captures c ON cc.capture_id = c.id
JOIN twitch_users tu ON tu.twitch_user_id = cc.twitch_user_id
WHERE c.session_id = ? AND c.broadcaster_id = ?
GROUP BY d
ORDER BY cnt DESC
LIMIT 10
`, sessionID, broadcasterID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	topDays := make([]TopDay, 0, 10)
	for rows.Next() {
		var d sql.NullTime
		var cnt int64
		if err := rows.Scan(&d, &cnt); err != nil {
			return nil, err
		}
		if !d.Valid {
			continue
		}
		topDays = append(topDays, TopDay{
			Date:  d.Time.Format("2006-01-02"),
			Count: cnt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &SessionSummary{
		SessionUUID:   sessionUUID,
		TotalAccounts: total,
		TopDays:       topDays,
		GeneratedAt:   time.Now().UTC(),
	}, nil
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
