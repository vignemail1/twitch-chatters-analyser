# Architecture et conception

Ce document décrit l’architecture technique de **Twitch Chatters Analyser**.

---

## 1. Vue d’ensemble

L’application est découpée en **micro‑services Go** :

- `gateway` : interface web (HTML server‑rendered + endpoints JSON).
- `twitch-api` : client centralisé pour l’API Twitch Helix.
- `worker` : traitement asynchrone des captures / enrichissement des comptes.
- `analysis` : service d’agrégation et d’analyse des données.

Une base **MySQL** (InnoDB) sert de stockage principal pour :

- les utilisateurs,
- les sessions web,
- les sessions d’analyse,
- les captures et chatters,
- les métadonnées Twitch (users + historique de noms),
- les jobs de la file,
- les logs d’audit.

---

## 2. Services

### 2.1 gateway

**Rôle :**

- Exposé publiquement (via Traefik).
- Gère :
  - auth Twitch (flow OAuth2),
  - création/gestion des sessions web,
  - UI server‑rendered (HTML, CSS, JS minimal+jQuery),
  - création des sessions d’analyse,
  - création des jobs de capture,
  - export CSV/JSON (en s’appuyant sur `analysis`).

**Technos :**

- Go `net/http` + `html/template`.
- jQuery pour les appels AJAX.
- Apache ECharts pour les graphes.

**Responsabilités principales :**

- **Login / Logout :**
  - redirection vers Twitch pour l’auth,
  - réception du code,
  - appel à `twitch-api` pour échanger le code contre des tokens,
  - création ou mise à jour de l’utilisateur local,
  - création de la session web (token stocké côté serveur),
  - suppression de la session web et des tokens en logout/expiration.

- **Sessions d’analyse :**
  - création (statut `active`),
  - sauvegarde (statut `saved`),
  - chargement/reprise d’une session sauvegardée,
  - expiration (pilotée en tâche de fond).

- **Captures :**
  - déclenchement d’une capture de chatters pour une chaîne,
  - création d’un job en base,
  - affichage de l’état (nombre de chatters capturés, nouveaux comptes…).

- **Analyse / Export :**
  - affichage des stats (via `analysis`),
  - export CSV/JSON filtré par période.

---

### 2.2 twitch-api

**Rôle :**

- Point d’accès unique aux API Twitch Helix.
- Gère :
  - OAuth2 (échange code ↔ token, refresh optionnel),
  - appels `GET /helix/...` (chatters, users, moderated channels),
  - pagination et **rate limiting global**.

**Design :**

- Exposé en HTTP interne (non public).
- Utilise un client HTTP avec :
  - timeouts raisonnables,
  - gestion des erreurs 429 (backoff) et 5xx.
- Un composant interne gère le rate limit :
  - compteur de requêtes par fenêtre de temps,
  - `time.Ticker` + channel pour espacer les appels.

**Endpoints internes (exemples) :**

- `POST /oauth/token`
- `GET /users/moderated-channels?user_id=...`
- `GET /chat/chatters?broadcaster_id=...&moderator_id=...`
- `GET /users?ids=...`

---

### 2.3 worker

**Rôle :**

- Consommer une file de jobs en base :
  - `FETCH_CHATTERS` : capturer les chatters d’une chaîne pour une session.
  - `FETCH_USERS_INFO` : enrichir les comptes Twitch en DB.

**Fonctionnement :**

- Boucle principale :
  - sélectionne un job `pending` avec verrou (SKIP LOCKED),
  - passe en `running`,
  - exécute la logique,
  - passe en `done` ou `failed`.
- Appelle `twitch-api` pour toutes les interactions avec Twitch.
- Gère :
  - insertion dans `captures` et `capture_chatters`,
  - mise à jour de `twitch_users`,
  - historisation des changements de `login`/`display_name` dans `twitch_user_names`.

**Rate limiting :**

- Délégué à `twitch-api`.  
- Le worker peut ainsi être multiplié en plusieurs instances sans casser les limites globales.

---

### 2.4 analysis

**Rôle :**

- Fournir des agrégations et analyses sur les données stockées.
- Endpoints internes : JSON ou CSV.

**Fonctionnalités :**

- **Résumé de session :**
  - nombre de comptes distincts,
  - nombre de captures,
  - date de dernière capture.

- **Top 10 jours de création :**
  - group by `DATE(twitch_users.created_at)`,
  - tri décroissant par count,
  - limit 10.

- **Filtres de période :**
  - `last_7_days`, `last_30_days`, `last_90_days`, `all`.

- **Exports :**
  - CSV et JSON de la distribution par jour/mois.

- **Analyses avancées (ultérieur) :**
  - comptes avec beaucoup de renommages,
  - scores de suspicion.

---

## 3. Modèle de données (résumé)

### 3.1 Tables principales

- `users` : utilisateurs de l’app (modérateurs).
- `web_sessions` : sessions web + tokens Twitch.
- `sessions` : sessions d’analyse.
- `broadcasters` : chaînes Twitch.
- `session_broadcasters` : lien sessions ↔ broadcasters (N:N).
- `twitch_users` : infos globales sur les comptes Twitch.
- `twitch_user_names` : historique des noms.
- `captures` : snapshots de chatters (par session/broadcaster).
- `capture_chatters` : lien capture ↔ compte Twitch.
- `jobs` : file de jobs pour le worker.
- `audit_logs` : journal d’audit.

Un schéma SQL détaillé sera ajouté dans `dev/schema.sql` lorsque le modèle sera stabilisé.

---

## 4. Flux principaux

### 4.1 Authentification

1. L’utilisateur clique sur « Se connecter avec Twitch ».
2. `gateway` redirige vers Twitch OAuth.
3. Twitch redirige vers `/auth/callback` avec `code`.
4. `gateway` appelle `twitch-api /oauth/token`.
5. `gateway` crée/maj l’utilisateur et la `web_session`, stocke les tokens dans `web_sessions`.
6. `gateway` loggue `auth_login`.

### 4.2 Création et utilisation d’une session d’analyse

1. L’utilisateur demande la création d’une session → `gateway` crée une entrée dans `sessions` (status `active`).
2. `gateway` affiche la liste des broadcasters (via `twitch-api` ou DB).
3. L’utilisateur clique sur « Capturer les chatters » pour un broadcaster :
   - `gateway` crée un `job` de type `FETCH_CHATTERS`.
4. `worker` traite le job :
   - appelle `twitch-api /chat/chatters`,
   - enregistre la capture + chatters,
   - crée un job `FETCH_USERS_INFO` pour les IDs inconnus.
5. `worker` traite `FETCH_USERS_INFO` :
   - appelle `twitch-api /users`,
   - met à jour `twitch_users` + `twitch_user_names`.
6. `gateway` et `analysis` peuvent ensuite afficher les stats pour cette session.

### 4.3 Expiration et sauvegarde de session

- Un job périodique (cron ou service dédié) :
  - marque les sessions `active` dont `expires_at` est dépassé en `expired`,
  - purge les captures liées, sauf si la session est `saved`.
- L’utilisateur peut cliquer sur « Sauvegarder la session » :
  - `gateway` met à jour le statut,
  - les données ne sont pas purgées automatiquement.

---

## 5. Logging et audit

Chaque service écrit :

- des logs applicatifs en stdout (format structuré),
- des événements métier dans `audit_logs`.

Événements principaux :

- `auth_login`, `auth_logout`, `web_session_expire`,
- `session_create`, `session_save`, `session_load`, `session_expire`,
- `capture_request`, `capture_complete`,
- `export_csv`, `export_json`,
- `user_rename_detected`.

---

## 6. Organisation du dépôt

Arborescence cible :

```text
.
├── cmd/
│   ├── gateway/
│   ├── twitch-api/
│   ├── worker/
│   └── analysis/
├── internal/
│   ├── db/
│   ├── models/
│   ├── twitch/
│   ├── jobs/
│   └── logging/
├── dev/
│   ├── architecture.md
│   ├── schema.sql        # (à venir)
│   └── example.env       # (à venir)
├── web/
│   ├── templates/
│   └── static/
├── docker-compose.yml    # (à venir)
└── README.md
```

Ce document pourra évoluer au fur et à mesure du développement.
