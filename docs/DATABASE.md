# Base de Données

Documentation de la structure de base de données et des migrations.

## Schema

Le schéma principal est défini dans `dev/schema.sql` et initialisé automatiquement au démarrage du container MariaDB.

**Version** : MariaDB 11.2  
**Charset** : `utf8mb4` avec collation `utf8mb4_unicode_ci`  
**Engine** : InnoDB

## Tables Principales

### users
Utilisateurs de l'application (modérateurs Twitch).

```sql
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
```

**Champs** :
- `twitch_user_id` : ID Twitch unique (immuable)
- `login` : Nom d'utilisateur Twitch (peut changer)
- `display_name` : Nom d'affichage (peut changer)
- `avatar_url` : URL de l'avatar
- `created_at` / `updated_at` : Horodatages avec microsecondes

### web_sessions
Sessions web avec tokens OAuth Twitch.

```sql
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
```

**Gestion des tokens** :
- Tokens OAuth stockés dans `web_sessions` (pas dans `users`)
- Expiration automatique via `expires_at`
- Refresh token optionnel

### sessions
Sessions d'analyse des chatters.

```sql
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
```

**Statuts** :
- `active` : Session en cours d'utilisation
- `saved` : Session sauvegardée par l'utilisateur (**max 10 par utilisateur**)
- `expired` : Session expirée automatiquement
- `deleted` : Session supprimée par l'utilisateur

**Limitation** : Maximum **10 sessions sauvegardées** par utilisateur. Les plus anciennes (basées sur `updated_at`) sont automatiquement supprimées via trigger (voir migration `001_limit_saved_sessions.sql`).

**Pas de system versioning** : Les tables n'utilisent **pas** `WITH SYSTEM VERSIONING`. L'historique est géré via `twitch_user_names` et `audit_logs`.

### captures
Snapshots de la liste des chatters à un instant T.

```sql
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
```

**Statistiques précalculées** :
- `chatters_count` : Nombre total de chatters dans cette capture
- `new_users_count` : Nombre de nouveaux chatters (première apparition dans la session)

### capture_chatters
Lien many-to-many entre captures et chatters.

```sql
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
```

**Volumétrie** : Table la plus volumineuse. Une capture de 1000 chatters = 1000 entrées.

### twitch_users
Cache des informations utilisateurs Twitch enrichies.

```sql
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
```

**Cache** : Informations enrichies depuis l'API Twitch, mises à jour par le worker.

### twitch_user_names
Historique des changements de noms (login/display_name).

```sql
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
```

**Usage** : Tracking des changements de pseudo/display name.

### jobs
File d'attente des jobs asynchrones pour le worker.

```sql
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
```

**Types de jobs** :
- `FETCH_CHATTERS` : Récupération de la liste des chatters
- `FETCH_USERS_INFO` : Enrichissement des données utilisateurs

### audit_logs
Table d'audit pour la traçabilité.

```sql
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
```

**Événements traçés** :
- Création/suppression de sessions
- Actions utilisateur importantes
- Erreurs système

## Migrations

Les migrations sont stockées dans `dev/migrations/` et doivent être appliquées manuellement.

### 001_limit_saved_sessions.sql

**But** : Limiter le nombre de sessions sauvegardées à 10 par utilisateur.

**Composants** :
1. **Procédure stockée** `cleanup_old_saved_sessions(user_id)` : Supprime les sessions les plus anciennes si > 10
2. **Trigger** `after_session_saved` : Exécute automatiquement le nettoyage après chaque sauvegarde
3. **Nettoyage initial** : Applique la limite aux données existantes

**Application** :

```bash
# Via mise (recommandé)
export MISE_ENV=production
docker exec -i twitch-chatters-db mariadb -u root -p"$MYSQL_ROOT_PASSWORD" twitch_chatters < dev/migrations/001_limit_saved_sessions.sql

# Ou via console MariaDB
mise run db-console
SOURCE /docker-entrypoint-initdb.d/migrations/001_limit_saved_sessions.sql;
```

**Vérification** :

```sql
-- Vérifier que le trigger existe
SHOW TRIGGERS LIKE 'sessions';

-- Vérifier que la procédure existe
SHOW PROCEDURE STATUS WHERE Name = 'cleanup_old_saved_sessions';

-- Tester manuellement
CALL cleanup_old_saved_sessions(1); -- Remplacer 1 par un user_id réel

-- Compter les sessions sauvegardées par utilisateur
SELECT user_id, COUNT(*) as saved_sessions
FROM sessions
WHERE status = 'saved'
GROUP BY user_id;
```

## Indexes

Indexes optimisés pour les requêtes fréquentes.

### Optimisation Performances

| Table | Index | Usage |
|-------|-------|-------|
| `users` | `idx_users_login` | Recherche par login |
| `web_sessions` | `idx_web_sessions_user` | Sessions par utilisateur |
| `web_sessions` | `idx_web_sessions_expires_at` | Nettoyage sessions expirées |
| `sessions` | `idx_sessions_user` | Recherche par utilisateur |
| `sessions` | `idx_sessions_status` | Filtrage par statut |
| `sessions` | `idx_sessions_user_status` | Combo user + status (getActiveSessionUUID) |
| `captures` | `idx_captures_session` | Captures d'une session |
| `captures` | `idx_captures_broadcaster` | Filtrage par broadcaster |
| `captures` | `idx_captures_session_captured` | Tri temporel |
| `capture_chatters` | `idx_capture_chatters_capture` | Chatters d'une capture |
| `capture_chatters` | `idx_capture_chatters_user` | Captures d'un user |
| `capture_chatters` | `idx_capture_chatters_capture_user` | Dédoublonnage |
| `jobs` | `idx_jobs_status_created` | Polling worker (CRITICAL) |
| `twitch_users` | `idx_twitch_users_login` | Recherche par login |
| `twitch_users` | `idx_twitch_users_created_at` | Tri par date création |
| `audit_logs` | `idx_audit_logs_event_type` | Filtrage par type |
| `audit_logs` | `idx_audit_logs_user` | Logs par utilisateur |
| `audit_logs` | `idx_audit_logs_created` | Tri temporel |

## Maintenance

### Backup

```bash
# Avec mise (utilise $APP_ENV automatiquement)
mise run db-backup
# Crée : backup-production-20260209-214700.sql

# Backup complet manuel
docker exec twitch-chatters-db mariadb-dump -u root -p"$MYSQL_ROOT_PASSWORD" twitch_chatters > backup.sql

# Backup sans les jobs (plus léger)
docker exec twitch-chatters-db mariadb-dump -u root -p"$MYSQL_ROOT_PASSWORD" twitch_chatters \
  --ignore-table=twitch_chatters.jobs \
  --ignore-table=twitch_chatters.audit_logs \
  > backup-minimal.sql
```

### Restauration

```bash
# Avec mise (avec confirmation)
mise run db-restore backup-production-20260209-214700.sql

# Restauration manuelle
docker exec -i twitch-chatters-db mariadb -u root -p"$MYSQL_ROOT_PASSWORD" twitch_chatters < backup.sql
```

### Nettoyage

```sql
-- Supprimer les jobs terminés de plus de 7 jours
DELETE FROM jobs 
WHERE status IN ('done', 'failed') 
  AND finished_at < NOW() - INTERVAL 7 DAY;

-- Supprimer les web_sessions expirées
DELETE FROM web_sessions 
WHERE expires_at < NOW();

-- Supprimer les sessions expirées de plus de 30 jours
DELETE FROM sessions 
WHERE status = 'expired' 
  AND updated_at < NOW() - INTERVAL 30 DAY;

-- Optimiser les tables
OPTIMIZE TABLE sessions, captures, capture_chatters, jobs;
```

### Statistiques

```sql
-- Taille des tables
SELECT 
    table_name,
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'Size (MB)',
    table_rows AS 'Rows'
FROM information_schema.TABLES
WHERE table_schema = 'twitch_chatters'
ORDER BY (data_length + index_length) DESC;

-- Nombre d'enregistrements par table
SELECT 
    'users' AS table_name, COUNT(*) AS count FROM users
UNION ALL
SELECT 'web_sessions', COUNT(*) FROM web_sessions
UNION ALL
SELECT 'sessions', COUNT(*) FROM sessions
UNION ALL
SELECT 'captures', COUNT(*) FROM captures
UNION ALL
SELECT 'capture_chatters', COUNT(*) FROM capture_chatters
UNION ALL
SELECT 'twitch_users', COUNT(*) FROM twitch_users
UNION ALL
SELECT 'jobs', COUNT(*) FROM jobs
UNION ALL
SELECT 'audit_logs', COUNT(*) FROM audit_logs;

-- Sessions par statut
SELECT status, COUNT(*) as count
FROM sessions
GROUP BY status;

-- Top 10 utilisateurs par nombre de captures
SELECT 
    u.login,
    COUNT(DISTINCT s.id) as sessions_count,
    COUNT(c.id) as captures_count,
    SUM(c.chatters_count) as total_chatters
FROM users u
LEFT JOIN sessions s ON s.user_id = u.id
LEFT JOIN captures c ON c.session_id = s.id
GROUP BY u.id
ORDER BY captures_count DESC
LIMIT 10;

-- Activité sur les dernières 24h
SELECT 
    DATE_FORMAT(captured_at, '%Y-%m-%d %H:00') as hour,
    COUNT(*) as captures_count,
    SUM(chatters_count) as total_chatters
FROM captures
WHERE captured_at >= NOW() - INTERVAL 24 HOUR
GROUP BY hour
ORDER BY hour;
```

## Performance

### Configuration MariaDB

Configuration actuelle dans `docker-compose.yml` :

```yaml
db:
  image: mariadb:11.2
  command: [
    "--character-set-server=utf8mb4",
    "--collation-server=utf8mb4_unicode_ci",
    "--max-connections=200",
    "--innodb-buffer-pool-size=512M"
  ]
```

### Monitoring

```sql
-- Connexions actives
SHOW PROCESSLIST;

-- Slow queries
SHOW GLOBAL STATUS LIKE 'Slow_queries';

-- Buffer pool hit rate (devrait être > 99%)
SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_read%';

-- Calcul du hit rate
SELECT 
    CONCAT(ROUND(
        (1 - (Innodb_buffer_pool_reads / Innodb_buffer_pool_read_requests)) * 100, 2
    ), '%') AS buffer_pool_hit_rate
FROM (
    SELECT 
        VARIABLE_VALUE AS Innodb_buffer_pool_reads
    FROM information_schema.GLOBAL_STATUS
    WHERE VARIABLE_NAME = 'Innodb_buffer_pool_reads'
) AS reads,
(
    SELECT 
        VARIABLE_VALUE AS Innodb_buffer_pool_read_requests
    FROM information_schema.GLOBAL_STATUS
    WHERE VARIABLE_NAME = 'Innodb_buffer_pool_read_requests'
) AS requests;

-- Taille du buffer pool utilisé
SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_bytes%';

-- Tables les plus gourmandes en I/O
SELECT 
    object_schema,
    object_name,
    count_read,
    count_write,
    count_fetch,
    count_insert,
    count_update,
    count_delete
FROM performance_schema.table_io_waits_summary_by_table
WHERE object_schema = 'twitch_chatters'
ORDER BY (count_read + count_write) DESC
LIMIT 10;
```

## Accès

### Console MariaDB

```bash
# Avec mise (utilise $MYSQL_ROOT_PASSWORD automatiquement)
mise run db-console

# Manuel
docker exec -it twitch-chatters-db mariadb -u root -p twitch_chatters
```

### Ports

**Development** : Port 3306 exposé pour debug  
**Production** : Port **NON exposé** (sécurité)

```bash
# Development uniquement
mysql -h 127.0.0.1 -P 3306 -u twitch -p twitch_chatters
```

## Sécurité

### Permissions

L'utilisateur `twitch` a uniquement les permissions nécessaires :

```sql
GRANT SELECT, INSERT, UPDATE, DELETE, EXECUTE ON twitch_chatters.* TO 'twitch'@'%';
```

**Pas de permissions** :
- `DROP` : Pas de suppression de tables
- `CREATE` : Pas de création de tables (sauf migrations root)
- `ALTER` : Pas de modification de structure

### Mots de passe

Mots de passe stockés via **fnox** (recommandé) ou `.env` (git ignored) :

```bash
# Avec fnox (sécurisé)
fnox secret set production MYSQL_ROOT_PASSWORD
fnox secret set production MYSQL_PASSWORD

# Ou dans .env.production
MYSQL_ROOT_PASSWORD=votre_mot_de_passe_fort
MYSQL_PASSWORD=votre_mot_de_passe_app
```

### Accès Externe

**Production** : MariaDB n'est **pas exposé** en dehors du réseau Docker backend (pas de `ports:` dans docker-compose.yml).

**Development** : Port 3306 exposé pour faciliter le debug local.

## Volu métrie Estimée

### Exemple : 1 utilisateur, 1 session, 100 captures, 1000 chatters/capture

| Table | Enregistrements | Taille estimée |
|-------|-----------------|------------------|
| `users` | 1 | ~1 KB |
| `web_sessions` | 1 | ~2 KB |
| `sessions` | 1 | ~500 B |
| `captures` | 100 | ~50 KB |
| `capture_chatters` | 100,000 | ~5 MB |
| `twitch_users` | 10,000 | ~2 MB |
| `jobs` | 200 | ~100 KB |
| **TOTAL** | | **~7.2 MB** |

### Projection : 100 utilisateurs actifs/mois

| Scénario | Captures/user | Chatters/capture | Taille BDD |
|----------|---------------|------------------|------------|
| Léger | 50 | 500 | ~1.5 GB |
| Moyen | 200 | 1000 | ~6 GB |
| Intensif | 1000 | 2000 | ~30 GB |

**Recommandation serveur** : 50-100 GB disque pour production.

## Ressources

- [MariaDB 11.2 Documentation](https://mariadb.com/kb/en/documentation/)
- [InnoDB Storage Engine](https://mariadb.com/kb/en/innodb/)
- [Optimizing Tables](https://mariadb.com/kb/en/optimization-and-indexes/)
- [JSON Support](https://mariadb.com/kb/en/json-functions/)
