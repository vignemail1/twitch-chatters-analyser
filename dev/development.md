# Guide de développement - Twitch Chatters Analyser

Ce document fournit toutes les informations nécessaires pour contribuer au projet.

---

## 🛠️ Environnement de développement

### Prérequis

- **Go 1.21+** pour le développement local
- **Docker & Docker Compose** pour l'exécution complète
- **MySQL 8.0+** (via Docker ou local)
- **Git** pour le versioning
- **Make** (optionnel, pour les raccourcis)

### Installation locale

```bash
# Cloner le dépôt
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# Copier la configuration
cp .env.example .env
# Éditer .env avec vos credentials Twitch

# Lancer uniquement la base de données
docker-compose up -d db

# Attendre l'initialisation de la DB (~10 secondes)
sleep 10

# Lancer les services en local
cd cmd/gateway && go run . &
cd cmd/worker && go run . &
cd cmd/analysis && go run . &
```

### Développement avec Docker

```bash
# Lancer tous les services
docker-compose up -d

# Voir les logs en temps réel
docker-compose logs -f

# Rebuilder après modification du code
docker-compose build gateway worker analysis
docker-compose restart gateway worker analysis
```

---

## 🏛️ Architecture des services

### Gateway (`cmd/gateway`)

**Responsabilités :**
- Authentification OAuth2 Twitch
- Gestion des sessions web (cookies)
- Interface utilisateur (HTML templates)
- Création de sessions d'analyse
- Émission de jobs pour le worker
- Export CSV/JSON

**Technologies :**
- `net/http` pour le serveur HTTP
- `html/template` pour le rendu côté serveur
- `database/sql` + driver MySQL
- Cookies sécurisés pour les sessions

**Endpoints principaux :**

| Route | Méthode | Description |
|-------|---------|-------------|
| `/` | GET | Page d'accueil |
| `/auth/login` | GET | Redirection OAuth2 Twitch |
| `/auth/callback` | GET | Callback OAuth2 |
| `/auth/logout` | GET | Déconnexion |
| `/channels` | GET | Liste des chaînes modérées |
| `/sessions/capture` | POST | Créer une capture |
| `/sessions/save` | POST | Sauvegarder la session active |
| `/sessions/purge` | POST | Supprimer la session active |
| `/sessions/delete` | POST | Supprimer une session sauvegardée |
| `/sessions` | GET | Liste des sessions sauvegardées |
| `/analysis` | GET | Analyse session active |
| `/analysis/saved/{uuid}` | GET | Analyse session sauvegardée |
| `/analysis/export` | GET | Export session active |
| `/sessions/export/{uuid}` | GET | Export session sauvegardée |

**Ajouter un endpoint :**

```go
// Dans cmd/gateway/main.go
func (a *App) handleNewFeature(w http.ResponseWriter, r *http.Request) {
    u := currentUser(r.Context())
    if u == nil {
        http.Redirect(w, r, "/auth/login", http.StatusFound)
        return
    }
    
    // Votre logique ici
    
    data := struct {
        Title       string
        CurrentUser *CurrentUser
        // Autres champs
    }{
        Title:       "Nouvelle fonctionnalité",
        CurrentUser: u,
    }
    
    if err := a.templates.ExecuteTemplate(w, "new_feature.html", data); err != nil {
        log.Printf("template error: %v", err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
    }
}

// Enregistrer dans main()
mux.HandleFunc("/new-feature", app.handleNewFeature)
```

---

### Worker (`cmd/worker`)

**Responsabilités :**
- Consommer la file de jobs
- Capturer les chatters via API Twitch
- Enrichir les profils utilisateurs
- Gérer les erreurs et retries

**Types de jobs :**

1. **FETCH_CHATTERS** - Capturer les viewers d'une chaîne
   ```json
   {
     "session_id": 123,
     "twitch_user_id": "12345",
     "broadcaster_id": "67890",
     "broadcaster_login": "streamer_name"
   }
   ```

2. **FETCH_USERS_INFO** - Enrichir les comptes Twitch
   ```json
   {
     "twitch_user_ids": ["123", "456", "789"]
   }
   ```

**Cycle de vie d'un job :**

```
pending → running → done
                    ↓
                  failed (avec retry possible)
```

**Ajouter un type de job :**

```go
// Dans cmd/worker/main.go

func (w *Worker) processJob(ctx context.Context, job Job) error {
    switch job.Type {
    case "FETCH_CHATTERS":
        return w.processFetchChatters(ctx, job)
    case "FETCH_USERS_INFO":
        return w.processFetchUsersInfo(ctx, job)
    case "NEW_JOB_TYPE": // Nouveau type
        return w.processNewJobType(ctx, job)
    default:
        return fmt.Errorf("unknown job type: %s", job.Type)
    }
}

func (w *Worker) processNewJobType(ctx context.Context, job Job) error {
    var payload struct {
        // Définir la structure du payload
        Field1 string `json:"field1"`
        Field2 int    `json:"field2"`
    }
    
    if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
        return fmt.Errorf("invalid payload: %w", err)
    }
    
    // Votre logique métier ici
    
    return nil
}
```

---

### Analysis (`cmd/analysis`)

**Responsabilités :**
- Calculer les statistiques agrégées
- Fournir le Top N des jours de création
- Gérer les filtres (broadcaster, période)
- Liste des broadcasters d'une session

**Endpoints :**

| Route | Méthode | Description |
|-------|---------|-------------|
| `/sessions/{uuid}/summary` | GET | Résumé d'une session |
| `/healthz` | GET | Health check |

**Paramètres de filtrage :**

- `broadcaster_id` - Filtrer par un ou plusieurs broadcasters (séparés par virgule)
  - Exemple : `?broadcaster_id=123,456,789`

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

**Ajouter une statistique :**

```go
// Dans cmd/analysis/main.go

// 1. Ajouter un champ à SessionSummary
type SessionSummary struct {
    SessionUUID   string        `json:"session_uuid"`
    TotalAccounts int64         `json:"total_accounts"`
    TopDays       []TopDay      `json:"top_days"`
    Broadcasters  []Broadcaster `json:"broadcasters"`
    NewStat       int64         `json:"new_stat"` // Nouveau champ
    GeneratedAt   time.Time     `json:"generated_at"`
}

// 2. Calculer dans buildSessionSummary
func (a *App) buildSessionSummary(...) (*SessionSummary, error) {
    // ... code existant ...
    
    // Calculer la nouvelle statistique
    var newStat int64
    err = a.db.QueryRowContext(ctx, `
        SELECT COUNT(*) FROM ...
        WHERE ...
    `).Scan(&newStat)
    if err != nil {
        return nil, err
    }
    
    return &SessionSummary{
        // ... champs existants ...
        NewStat: newStat,
    }, nil
}
```

---

## 💾 Base de données

### Schéma relationnel

Voir [schema.sql](schema.sql) pour le schéma complet.

**Relations principales :**

```
users (modérateurs)
  ↓ 1:N
web_sessions (tokens OAuth)
  ↓ 1:N
sessions (analyses)
  ↓ 1:N
captures (snapshots)
  ↓ N:M
accounts (comptes Twitch dédupliqués)
  ↓ 1:1
twitch_users (métadonnées enrichies)
  ↓ 1:N
twitch_user_names (historique des noms)
```

### Migrations

Pour l'instant, les migrations sont manuelles :

```bash
# Se connecter à MySQL
docker-compose exec db mysql -u twitch -ptwitchpass twitch_chatters

# Exécuter vos requêtes ALTER TABLE
ALTER TABLE twitch_users ADD COLUMN new_field VARCHAR(255);
```

**🚧 TODO :** Implémenter un système de migrations avec versioning (golang-migrate ou similaire).

### Requêtes utiles pour le développement

```sql
-- Voir les jobs en cours
SELECT * FROM jobs WHERE status = 'running';

-- Compter les jobs par statut
SELECT status, COUNT(*) FROM jobs GROUP BY status;

-- Voir les sessions actives
SELECT * FROM sessions WHERE status = 'active';

-- Compter les comptes enrichis
SELECT COUNT(*) FROM twitch_users WHERE created_at IS NOT NULL;

-- Top des jours de création (manuel)
SELECT 
    DATE(tu.created_at) AS creation_date,
    COUNT(DISTINCT tu.twitch_user_id) AS compte_count
FROM twitch_users tu
WHERE tu.created_at IS NOT NULL
GROUP BY DATE(tu.created_at)
ORDER BY compte_count DESC
LIMIT 20;

-- Voir l'historique des noms d'un utilisateur
SELECT * FROM twitch_user_names 
WHERE twitch_user_id = '123456789' 
ORDER BY changed_at DESC;
```

---

## 🎨 Frontend

### Templates HTML

Situés dans `web/templates/`, utilisant le moteur Go `html/template`.

**Structure :**

- `index.html` - Page d'accueil
- `channels.html` - Liste des chaînes modérées
- `analysis.html` - Page d'analyse (définie comme `analysis_page`)
- `sessions.html` - Liste des sessions sauvegardées

**Fonctions template disponibles :**

```go
// Définies dans cmd/gateway/main.go
funcMap := template.FuncMap{
    "add":      func(a, b int64) int64 { return a + b },
    "mul":      func(a, b int64) int64 { return a * b },
    "div":      func(a, b int64) int64 { return a / b },
    "contains": func(slice []string, item string) bool { ... },
}
```

**Ajouter un template :**

1. Créer `web/templates/new_page.html`
2. Définir avec `{{ define "new_page" }}`
3. Utiliser dans le handler : `a.templates.ExecuteTemplate(w, "new_page", data)`

**Exemple de template :**

```html
{{ define "new_page" }}
<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="utf-8" />
    <title>{{ .Title }}</title>
    <link href="/static/css/main.css" rel="stylesheet" />
</head>
<body class="dark">
<header>
    <h1>{{ .Title }}</h1>
    {{ if .CurrentUser }}
        <p>Connecté : {{ .CurrentUser.DisplayName }}</p>
    {{ end }}
</header>
<main>
    <!-- Votre contenu ici -->
</main>
</body>
</html>
{{ end }}
```

### CSS et JavaScript

Situés dans `web/static/`.

- `css/main.css` - Styles globaux (dark theme)
- `js/timezone.js` - Conversion des dates UTC vers timezone locale

**Ajouter du JavaScript :**

```html
<!-- Dans votre template -->
<script src="/static/js/your-script.js"></script>
```

### Timezone support

Le fichier `timezone.js` convertit automatiquement les dates :

```html
<!-- Dans le template -->
<span data-utc-date="{{ .Date.Format "2006-01-02T15:04:05Z07:00" }}" 
      data-format="datetime">
    {{ .Date.Format "02/01/2006 15:04" }}
</span>
```

Le JavaScript détecte ces éléments et les convertit au chargement.

---

## 🧪 Tests

### Tests unitaires

**🚧 TODO :** Implémenter les tests unitaires.

Structure proposée :

```
cmd/gateway/
  main.go
  main_test.go
  handlers_test.go
```

Exemple de test :

```go
func TestHandleAnalysis(t *testing.T) {
    // Setup
    app := &App{...}
    
    // Test
    req := httptest.NewRequest("GET", "/analysis", nil)
    w := httptest.NewRecorder()
    
    app.handleAnalysis(w, req)
    
    // Assertions
    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}
```

### Tests d'intégration

**🚧 TODO :** Créer une suite de tests d'intégration avec Docker Compose.

---

## 🔧 Débogage

### Logs structurés

Tous les services utilisent `log.Printf` pour les logs.

```bash
# Voir tous les logs
docker-compose logs -f

# Logs d'un service spécifique
docker-compose logs -f gateway
docker-compose logs -f worker

# Filtrer par pattern
docker-compose logs worker | grep ERROR
```

### Debug avec Delve

Pour debugger avec un debugger Go :

```bash
# Installer Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Lancer en mode debug
cd cmd/gateway
dlv debug

# Dans Delve
(dlv) break main.handleAnalysis
(dlv) continue
```

### Inspecter la base de données

```bash
# Shell MySQL interactif
docker-compose exec db mysql -u twitch -ptwitchpass twitch_chatters

# Exécuter une requête directement
docker-compose exec db mysql -u twitch -ptwitchpass -e "SELECT COUNT(*) FROM twitch_chatters.jobs;"
```

---

## 🚀 Déploiement

### Build des images Docker

```bash
# Build tous les services
docker-compose build

# Build un service spécifique
docker-compose build gateway

# Build sans cache
docker-compose build --no-cache
```

### Variables d'environnement

Toutes les variables sont dans `.env` :

```bash
# Application
APP_ENV=development
APP_SESSION_SECRET=change-me

# Twitch OAuth
TWITCH_CLIENT_ID=your_client_id
TWITCH_CLIENT_SECRET=your_client_secret
TWITCH_REDIRECT_URL=http://localhost:8080/auth/callback

# MySQL
MYSQL_ROOT_PASSWORD=rootpass
DB_USER=twitch
DB_PASSWORD=twitchpass
DB_HOST=db
DB_PORT=3306
DB_NAME=twitch_chatters

# Services
GATEWAY_PORT=8080
WORKER_PORT=8081
ANALYSIS_PORT=8083
ANALYSIS_BASE_URL=http://analysis:8083
```

### Checklist production

- [ ] Changer tous les mots de passe
- [ ] Activer HTTPS
- [ ] Configurer `Secure: true` sur les cookies
- [ ] Limiter l'accès MySQL (pas d'exposition publique)
- [ ] Configurer un reverse proxy (Traefik/Nginx)
- [ ] Mettre en place les sauvegardes automatiques
- [ ] Configurer la rotation des logs
- [ ] Monitorer les ressources (CPU, RAM, disque)
- [ ] Tester le processus de restauration

---

## 📝 Conventions de code

### Go

- **Formatting** : Utiliser `gofmt` (ou `goimports`)
- **Linting** : Passer `golangci-lint`
- **Naming** :
  - Variables : `camelCase`
  - Fonctions exportées : `PascalCase`
  - Constantes : `PascalCase` ou `SCREAMING_SNAKE_CASE`

```bash
# Formater le code
go fmt ./...

# Linter
golangci-lint run
```

### SQL

- Toujours utiliser des prepared statements (`?` placeholders)
- Jamais de concaténation de strings SQL
- INDEX sur les colonnes souvent filtrées

```go
// ✅ BON
rows, err := db.Query("SELECT * FROM users WHERE id = ?", userID)

// ❌ MAUVAIS (injection SQL)
rows, err := db.Query("SELECT * FROM users WHERE id = " + userID)
```

### Commits

Utiliser [Conventional Commits](https://www.conventionalcommits.org/) :

- `feat:` Nouvelle fonctionnalité
- `fix:` Correction de bug
- `docs:` Documentation
- `refactor:` Refactoring sans changement fonctionnel
- `test:` Ajout/modification de tests
- `chore:` Maintenance (dépendances, config, etc.)

```bash
git commit -m "feat: ajout export CSV avec filtres"
git commit -m "fix: correction timezone sur page analysis"
git commit -m "docs: mise à jour README avec nouvelles features"
```

---

## 🔗 Ressources utiles

### Documentation externe

- [API Twitch Helix](https://dev.twitch.tv/docs/api/reference)
- [OAuth2 Twitch](https://dev.twitch.tv/docs/authentication/getting-tokens-oauth)
- [Go html/template](https://pkg.go.dev/html/template)
- [MySQL 8.0 Reference](https://dev.mysql.com/doc/refman/8.0/en/)
- [Docker Compose](https://docs.docker.com/compose/)

### Outils recommandés

- **IDE** : VS Code avec extension Go
- **API Testing** : Postman ou curl
- **MySQL GUI** : DBeaver, MySQL Workbench, ou Adminer
- **Git GUI** : GitKraken, Sourcetree, ou ligne de commande

---

## ❓ FAQ Développeurs

**Q: Comment ajouter un nouveau champ à une table ?**

R: Exécutez un `ALTER TABLE` via MySQL, puis mettez à jour les structs Go et requêtes SQL.

**Q: Comment redémarrer un service sans tout redémarrer ?**

R: `docker-compose restart gateway` (ou worker/analysis)

**Q: Le worker ne traite pas mes jobs, pourquoi ?**

R: Vérifiez les logs (`docker-compose logs worker`), le statut des jobs en DB, et que le worker tourne bien.

**Q: Comment accéder au container pour debugger ?**

R: `docker-compose exec gateway sh` (ou bash si disponible)

**Q: Comment vider complètement la base de données ?**

R: 
```bash
docker-compose down -v  # Supprime les volumes
docker-compose up -d    # Recrée tout
```

---

**Besoin d'aide ?** Ouvrez une [issue](https://github.com/vignemail1/twitch-chatters-analyser/issues) ! 🚀
