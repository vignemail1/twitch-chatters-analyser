-- Utilisateur de l'application (modérateur)
CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    twitch_user_id VARCHAR(64) NOT NULL UNIQUE,
    login VARCHAR(128) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    avatar_url VARCHAR(255) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_users_login (login)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Session web (tokens Twitch stockés ici, pas dans users)
CREATE TABLE IF NOT EXISTS web_sessions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    session_id CHAR(36) NOT NULL UNIQUE, -- UUID
    user_id BIGINT UNSIGNED NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT NULL,
    scopes TEXT NULL,
    created_at DATETIME(6) NOT NULL,
    last_activity_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_web_sessions_user
        FOREIGN KEY (user_id) REFERENCES users(id)
        ON DELETE CASCADE,
    INDEX idx_web_sessions_user (user_id),
    INDEX idx_web_sessions_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Table des sessions d'analyse (minimal, on enrichira plus tard)
CREATE TABLE IF NOT EXISTS sessions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    session_uuid CHAR(36) NOT NULL UNIQUE,
    user_id BIGINT UNSIGNED NOT NULL,
    status ENUM('active','saved','expired','deleted') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_sessions_user (user_id),
    INDEX idx_sessions_status (status),
    INDEX idx_sessions_user_status (user_id, status) -- Optimisation pour getActiveSessionUUID
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Captures (snapshots de chatters)
CREATE TABLE IF NOT EXISTS captures (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    session_id BIGINT UNSIGNED NOT NULL,
    broadcaster_id VARCHAR(64) NOT NULL,
    broadcaster_login VARCHAR(128) NOT NULL,
    captured_at DATETIME(6) NOT NULL,
    chatters_count INT NOT NULL DEFAULT 0,
    new_users_count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    CONSTRAINT fk_captures_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    INDEX idx_captures_session (session_id),
    INDEX idx_captures_broadcaster (broadcaster_id),
    INDEX idx_captures_session_captured (session_id, captured_at) -- Optimisation pour analyses temporelles
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Lien capture -> chatters (sera utilisé par le worker plus tard)
CREATE TABLE IF NOT EXISTS capture_chatters (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    capture_id BIGINT UNSIGNED NOT NULL,
    twitch_user_id VARCHAR(64) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_capture_chatters_capture FOREIGN KEY (capture_id) REFERENCES captures(id) ON DELETE CASCADE,
    INDEX idx_capture_chatters_capture (capture_id),
    INDEX idx_capture_chatters_user (twitch_user_id),
    INDEX idx_capture_chatters_capture_user (capture_id, twitch_user_id) -- Optimisation pour dédoublonnage
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- File de jobs pour le worker
CREATE TABLE IF NOT EXISTS jobs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    type VARCHAR(64) NOT NULL, -- ex: FETCH_CHATTERS, FETCH_USERS_INFO
    payload JSON NOT NULL,
    status ENUM('pending','running','done','failed') NOT NULL DEFAULT 'pending',
    created_at DATETIME(6) NOT NULL,
    started_at DATETIME(6) NULL,
    finished_at DATETIME(6) NULL,
    error_message TEXT NULL,
    PRIMARY KEY (id),
    INDEX idx_jobs_status_created (status, created_at) -- Optimisation pour polling worker
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Comptes Twitch (enrichis par le worker)
CREATE TABLE IF NOT EXISTS twitch_users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    twitch_user_id VARCHAR(64) NOT NULL UNIQUE,
    login VARCHAR(128) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    created_at DATETIME(6) NULL,
    broadcaster_type VARCHAR(32) NULL,
    type VARCHAR(32) NULL,
    view_count INT NULL,
    last_fetched_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_twitch_users_login (login),
    INDEX idx_twitch_users_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Historique des changements de noms (login/display_name)
CREATE TABLE IF NOT EXISTS twitch_user_names (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    twitch_user_id VARCHAR(64) NOT NULL,
    login VARCHAR(128) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    detected_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_twitch_user_names_user (twitch_user_id),
    INDEX idx_twitch_user_names_detected (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Table d'audit pour la traçabilité
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    event_type VARCHAR(64) NOT NULL,
    user_id BIGINT UNSIGNED NULL,
    session_id BIGINT UNSIGNED NULL,
    details JSON NULL,
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_audit_logs_event_type (event_type),
    INDEX idx_audit_logs_user (user_id),
    INDEX idx_audit_logs_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
