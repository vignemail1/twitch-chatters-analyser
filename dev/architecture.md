# Architecture et conception

Ce document décrit l'architecture technique de **Twitch Chatters Analyser**.

---

## 1. Vue d'ensemble

L'application est découpée en **micro‑services Go** :

- `gateway` : interface web (HTML server‑rendered + endpoints JSON).
- `worker` : traitement asynchrone des captures / enrichissement des comptes.
- `analysis` : service d'agrégation et d'analyse des données.
- `twitch-api` : *(en développement)* client centralisé pour l'API Twitch Helix.

Une base **MySQL** (InnoDB) sert de stockage principal pour :

- les utilisateurs,
- les sessions web,
- les sessions d'analyse,
- les captures et chatters,
- les métadonnées Twitch (users + historique de noms),
- les jobs de la file,
- les comptes dédupliqués.

---

## 2. Services

### 2.1 gateway

**Rôle :**

- Exposé publiquement (via reverse proxy).
- Gère :
  - auth Twitch (flow OAuth2),
  - création/gestion des sessions web,
  - UI server‑rendered (HTML templates Go),
  - création des sessions d'analyse,
  - création des jobs de capture,
  - export CSV/JSON (via le service analysis),
  - filtrage multi-broadcaster.

**Technos :**

- Go `net/http` + `html/template`.
- JavaScript vanilla pour timezone conversion.
- MySQL via `database/sql`.

**Responsabilités principales :**

- **Login / Logout :**
  - redirection vers Twitch pour l'auth,
  - réception du code,
  - échange du code contre des tokens via API Twitch,
  - création ou mise à jour de l'utilisateur local,
  - création de la session web (token stocké côté serveur),
  - suppression de la session web et des tokens en logout/expiration.

- **Sessions d'analyse :**
  - création (statut `active`),
  - sauvegarde (statut `saved`),
  - chargement/reprise d'une session sauvegardée,
  - expiration (pilotée manuellement ou via purge).

- **Captures :**
  - déclenchement d'une capture de chatters pour une chaîne,
  - création d'un job en base,
  - affichage de l'état (nombre de chatters capturés).

- **Analyse / Export :**
  - affichage des stats (via `analysis`),
  - filtrage par broadcaster(s) avec cases à cocher,
  - export CSV/JSON filtré.

**Nouveautés récentes :**

- ✅ Export CSV/JSON depuis sessions actives et sauvegardées
- ✅ Filtrage multi-broadcaster avec UI à cases à cocher
- ✅ Conversion timezone navigateur pour affichage dates locales
- ✅ Liste des broadcasters dans le résumé de session

---

### 2.2 worker

**Rôle :**

- Consommer une file de jobs en base :
  - `FETCH_CHATTERS` : capturer les chatters d'une chaîne pour une session.
  - `FETCH_USERS_INFO` : enrichir les comptes Twitch en DB.

**Fonctionnement :**

- Boucle principale :
  - sélectionne un job `pending` avec verrou (`FOR UPDATE SKIP LOCKED`),
  - passe en `running`,
  - exécute la logique,
  - passe en `done` ou `failed`.
- Appelle directement l'API Twitch Helix (pour l'instant).
- Gère :
  - insertion dans `captures` et `capture_chatters`,
  - upsert dans `accounts` (déduplication),
  - mise à jour de `twitch_users`,
  - historisation des changements de `login`/`display_name` dans `twitch_user_names` *(en développement)*.

**Rate limiting :**

- Actuellement : délai fixe entre les appels Twitch.
- 🚧 **TODO** : déléguer au service `twitch-api` avec rate limiting global.

**Améliorations prévues :**

- 🔄 Détection et stockage des changements de noms
- 🔄 Enrichissement parallélisé par batch de 100 users
- 🔄 Retry intelligent avec backoff exponentiel

---

### 2.3 analysis

**Rôle :**

- Fournir des agrégations et analyses sur les données stockées.
- Endpoints internes : JSON.

**Fonctionnalités :**

- **Résumé de session :**
  - nombre de comptes distincts (avec ou sans filtre broadcaster),
  - liste des broadcasters présents dans la session,
  - top 10 des jours de création de comptes,
  - timestamp de génération.

- **Filtrage multi-broadcaster :**
  - support de paramètre `broadcaster_id` avec valeurs multiples (séparées par virgule),
  - requêtes SQL dynamiques avec `IN` clause,
  - calculs ajustés selon le filtre actif.

- **Analyses avancées (à venir) :**
  - comptes avec beaucoup de renommages,
  - score de suspicion automatique,
  - patterns temporels suspects.

**Endpoint principal :**

```
GET /sessions/{uuid}/summary?broadcaster_id=123,456
```

**Réponse JSON :**

```json
{
  "session_uuid": "abc123",
  "total_accounts": 1234,
  "top_days": [
    {"date": "2024-01-15", "count": 150},
    {"date": "2024-01-10", "count": 120}
  ],
  "broadcasters": [
    {
      "broadcaster_id": "123",
      "broadcaster_login": "streamer1",
      "capture_count": 5
    }
  ],
  "generated_at": "2024-02-09T00:00:00Z"
}
```

---

### 2.4 twitch-api *(en développement)*

**Rôle :**

- Point d'accès unique aux API Twitch Helix.
- Gère :
  - OAuth2 (échange code ↔ token, refresh optionnel),
  - appels `GET /helix/...` (chatters, users, moderated channels),
  - pagination et **rate limiting global**,
  - cache avec Redis (optionnel).

**Design :**

- Exposé en HTTP interne (non public).
- Utilise un client HTTP avec :
  - timeouts raisonnables,
  - gestion des erreurs 429 (backoff) et 5xx.
- Un composant interne gère le rate limit :
  - compteur de requêtes par fenêtre de temps,
  - `time.Ticker` + channel pour espacer les appels.

**Endpoints internes (proposés) :**

- `POST /oauth/token`
- `GET /users/moderated-channels?user_id=...`
- `GET /chat/chatters?broadcaster_id=...&moderator_id=...`
- `GET /users?ids=...` (batch jusqu'à 100 IDs)

**Priorité :** 🟡 Moyenne (actuellement appels directs depuis gateway/worker)

---

## 3. Modèle de données

Voir [schema.sql](schema.sql) pour le schéma SQL complet.

### 3.1 Tables principales

**Utilisateurs et authentification :**

- `users` : utilisateurs de l'app (modérateurs Twitch).
  - Colonnes : `id`, `twitch_user_id`, `login`, `display_name`, `avatar_url`, timestamps.
- `web_sessions` : sessions web + tokens Twitch.
  - Colonnes : `session_id` (UUID), `user_id`, `access_token`, `refresh_token`, `scopes`, `expires_at`.

**Sessions d'analyse :**

- `sessions` : sessions d'analyse.
  - Colonnes : `id`, `session_uuid`, `user_id`, `status` (active/saved/deleted), timestamps.
  - Une session contient plusieurs captures de différentes chaînes.

**Captures :**

- `captures` : snapshots de chatters.
  - Colonnes : `id`, `session_id`, `broadcaster_id`, `broadcaster_login`, `captured_at`, `chatter_count`.
- `capture_chatters` : lien N:M entre captures et accounts.
  - Colonnes : `capture_id`, `account_id`, `twitch_user_id`.

**Comptes Twitch :**

- `accounts` : comptes Twitch dédupliqués (un compte = un `twitch_user_id`).
  - Colonnes : `id`, `twitch_user_id` (UNIQUE), `login`, `display_name`, timestamps.
- `twitch_users` : métadonnées enrichies des comptes.
  - Colonnes : `twitch_user_id`, `created_at`, `profile_image_url`, `description`, etc.
  - Relation 1:1 avec `accounts`.
- `twitch_user_names` : historique des renommages *(en cours d'implémentation)*.
  - Colonnes : `id`, `twitch_user_id`, `old_login`, `new_login`, `changed_at`.

**Jobs :**

- `jobs` : file d'attente pour le worker.
  - Colonnes : `id`, `type`, `payload` (JSON), `status`, `attempts`, timestamps.

### 3.2 Relations

```
users (1) ——— (N) web_sessions
users (1) ——— (N) sessions
sessions (1) ——— (N) captures
captures (N) ——— (M) accounts  [via capture_chatters]
accounts (1) ——— (1) twitch_users
twitch_users (1) ——— (N) twitch_user_names
```

### 3.3 Index importants

```sql
-- Recherche de sessions par utilisateur
INDEX idx_sessions_user_status ON sessions(user_id, status);

-- Filtrage des captures par broadcaster
INDEX idx_captures_session_broadcaster ON captures(session_id, broadcaster_id);

-- Lookup rapide des comptes
UNIQUE INDEX idx_accounts_twitch_user_id ON accounts(twitch_user_id);

-- Analyse temporelle des créations
INDEX idx_twitch_users_created ON twitch_users(created_at);

-- Polling des jobs
INDEX idx_jobs_status_created ON jobs(status, created_at);
```

---

## 4. Flux principaux

### 4.1 Authentification

```
1. User clique "Se connecter avec Twitch"
   ↓
2. Gateway redirige vers Twitch OAuth
   (scopes: user:read:moderated_channels, moderator:read:chatters)
   ↓
3. Twitch redirige vers /auth/callback avec code
   ↓
4. Gateway échange code contre tokens (API Twitch)
   ↓
5. Gateway crée/maj user + web_session
   ↓
6. Cookie tca_session posé
   ↓
7. Redirect vers /
```

### 4.2 Création et utilisation d'une session d'analyse

```
1. User demande /channels
   ↓
2. Gateway crée session (status=active) si inexistante
   ↓
3. Gateway affiche liste broadcasters (API Twitch)
   ↓
4. User clique "Capturer chatters" pour broadcaster X
   ↓
5. Gateway crée job FETCH_CHATTERS
   ↓
6. Worker traite job:
   - Appelle API Twitch /chat/chatters
   - Crée capture + capture_chatters
   - Upsert dans accounts
   - Crée job FETCH_USERS_INFO pour IDs inconnus
   ↓
7. Worker traite FETCH_USERS_INFO:
   - Appelle API Twitch /users (batch 100)
   - Upsert dans twitch_users
   - Détecte changements de noms (TODO)
   ↓
8. User va sur /analysis
   ↓
9. Gateway appelle Analysis /sessions/{uuid}/summary
   ↓
10. Analysis calcule stats et retourne JSON
    ↓
11. Gateway affiche résultats avec filtres broadcaster
```

### 4.3 Export de données

```
1. User clique "Exporter CSV" sur /analysis
   ↓
2. Gateway requête directe MySQL:
   SELECT accounts + métadonnées pour session_id
   ↓
3. Gateway génère CSV en streaming
   ↓
4. Header Content-Disposition: attachment
   ↓
5. Browser télécharge fichier session_{uuid}.csv
```

Format CSV :
```csv
twitch_user_id,login,display_name,created_at,seen_count,first_seen,last_seen
123456,user1,User1,2020-01-15T10:30:00Z,3,2024-02-01T...,2024-02-08T...
```

### 4.4 Sauvegarde et purge de session

**Sauvegarde :**

```
1. User clique "Sauvegarder la session"
   ↓
2. Gateway: UPDATE sessions SET status='saved'
   ↓
3. Session préservée indéfiniment
   ↓
4. Redirect vers /sessions (liste)
```

**Purge :**

```
1. User clique "Purger la session" (avec confirmation)
   ↓
2. Gateway:
   - DELETE capture_chatters (via JOIN)
   - DELETE captures
   - UPDATE sessions SET status='deleted'
   ↓
3. Session vidée mais structure conservée
   ↓
4. Redirect vers /channels
```

---

## 5. Sécurité

### 5.1 Authentification

- **OAuth2 Twitch** : seule méthode d'auth.
- **Tokens stockés** : en DB, chiffrés au repos (TODO: encryption at rest).
- **Sessions web** : UUID aléatoire, expiration 24h.
- **Cookies** : `HttpOnly`, `SameSite=Lax`, `Secure=true` en production.

### 5.2 Autorisations

- **Scopes Twitch requis** :
  - `user:read:moderated_channels` - Liste des chaînes où l'utilisateur est modérateur.
  - `moderator:read:chatters` - Lecture des chatters (nécessite modération).

- **Vérifications** :
  - Chaque handler vérifie `currentUser(ctx)`.
  - Sessions appartiennent à l'utilisateur connecté.
  - Impossible d'accéder aux sessions d'un autre user.

### 5.3 Injection SQL

- **Toujours** utiliser prepared statements (`?` placeholders).
- **Jamais** de concaténation de strings SQL.
- Échappement automatique par le driver MySQL.

### 5.4 Rate Limiting

- **Twitch API** : limites respectées par délais entre requêtes.
- **Gateway** : TODO rate limiting par IP/user.
- **Worker** : une instance = traitement séquentiel (pas de parallélisme pour l'instant).

---

## 6. Logging et monitoring

### 6.1 Logs applicatifs

Chaque service écrit en `stdout` :

```
2024/02/09 00:00:00 gateway listening on :8080
2024/02/09 00:01:23 GET /analysis from 172.18.0.1 in 45ms
2024/02/09 00:02:15 session 123 saved by user 456
```

Format : timestamp + message libre (pas de JSON structuré pour l'instant).

### 6.2 Monitoring (TODO)

- **Métriques** : Prometheus + Grafana
  - Requêtes HTTP (latence, codes)
  - Jobs traités (succès/échecs)
  - Taille de la queue
  - Utilisation DB (connexions, slow queries)

- **Alerting** : Alertmanager
  - Worker arrêté > 5min
  - Queue jobs > 1000
  - Erreurs 5xx > 10/min

---

## 7. Performance

### 7.1 Optimisations actuelles

- **Index MySQL** sur colonnes filtrées fréquemment.
- **Connection pooling** : `SetMaxOpenConns(10)` par service.
- **Pagination** : limitée à 10 résultats (top days).
- **Deduplication** : table `accounts` évite doublons.

### 7.2 Améliorations futures

- **Cache Redis** :
  - Résultats d'analyse (TTL 5min).
  - Liste broadcasters (TTL 1h).
  - Profils utilisateurs Twitch (TTL 24h).

- **Batch processing** :
  - Enrichissement par batch de 100 users (API Twitch).
  - Insertion bulk dans `twitch_users`.

- **Compression** :
  - Gzip sur réponses JSON > 1KB.
  - Export CSV streamé (déjà implémenté).

---

## 8. Organisation du dépôt

Arborescence actuelle :

```text
.
├── cmd/
│   ├── gateway/      # Service web principal
│   │   └── main.go
│   ├── worker/       # Traitement asynchrone
│   │   └── main.go
│   ├── analysis/     # Service d'analyse
│   │   └── main.go
│   └── twitch-api/   # (TODO) Proxy rate-limité
├── web/
│   ├── static/
│   │   ├── css/
│   │   │   └── main.css
│   │   └── js/
│   │       └── timezone.js
│   └── templates/
│       ├── index.html
│       ├── channels.html
│       ├── analysis.html
│       └── sessions.html
├── dev/
│   ├── architecture.md    # Ce document
│   ├── development.md     # Guide développeur
│   └── schema.sql         # Schéma MySQL
├── docker-compose.yml
├── .env.example
├── .gitignore
└── README.md
```

**Absence de `internal/` :**  
Pour l'instant, tout le code est dans les `cmd/`. Si le projet grandit, envisager de factoriser les packages communs :

```text
internal/
  ├── db/         # Helpers DB communs
  ├── models/     # Structs partagées
  ├── twitch/     # Client API Twitch
  └── auth/       # Logique OAuth2
```

---

## 9. Roadmap technique

### Phase 1 : Stabilisation (en cours)

- [x] Architecture multi-services fonctionnelle
- [x] Authentification OAuth2 Twitch
- [x] Capture et enrichissement des chatters
- [x] Analyse de base (top 10 jours)
- [x] Export CSV/JSON
- [x] Filtrage multi-broadcaster
- [x] Timezone navigateur

### Phase 2 : Détection intelligente (prochain)

- [ ] Historique des changements de noms
- [ ] Score de suspicion automatique
- [ ] Détection de patterns temporels
- [ ] Alertes visuelles avancées

### Phase 3 : Infrastructure robuste

- [ ] Service twitch-api avec rate limiting
- [ ] Cache Redis
- [ ] Métriques Prometheus
- [ ] Tests unitaires et d'intégration
- [ ] CI/CD (GitHub Actions)

### Phase 4 : Fonctionnalités avancées

- [ ] Graphiques interactifs (Chart.js)
- [ ] Comparaison entre captures
- [ ] Notifications Discord/Slack
- [ ] API REST publique
- [ ] Authentification 2FA

---

Ce document évoluera avec le projet. Dernière mise à jour : **Février 2026**.
