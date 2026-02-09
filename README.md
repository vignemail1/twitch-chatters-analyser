# Twitch Chatters Analyser

Application d'analyse des spectateurs Twitch pour modérateurs et streamers.

## 📊 Vue d'ensemble

Twitch Chatters Analyser permet aux modérateurs de chaînes Twitch de :
- 📋 Capturer périodiquement la liste des chatters actifs
- 📈 Analyser les statistiques de participation
- 🔍 Identifier les nouveaux spectateurs
- 📊 Visualiser l'évolution dans le temps
- 💾 Exporter les données pour analyses avancées

## 🏛️ Architecture

### Services

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                                                                                  │
│                                   INFRASTRUCTURE                                               │
│                                                                                                  │
│  ┌───────────────┐   ┌───────────────┐   ┌───────────────┐   ┌───────────────┐  │
│  │   Gateway     │   │  Twitch-API  │   │   Worker      │   │  Analysis    │  │
│  │  HTTP API    │   │  Rate Limit  │   │  Job Queue   │   │  Stats       │  │
│  └───────┬────────┘   └───────┬────────┘   └───────┬────────┘   └───────┬────────┘  │
│          │                   │               │               │             │
│          v                   v               v               v             │
│  ┌───────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                                                                            │  │
│  │                 ┌─────────────────────────────────────────┐                     │  │
│  │                 │         Backend Network              │                     │  │
│  │                 └────────────────┬────────────────────────┘                     │  │
│  │                                  │                                                 │  │
│  │                     ┌────────────┼────────────┐                                   │  │
│  │                     │             │            │                                   │  │
│  │                ┌────v───────────── ┌────v───────────┐                              │  │
│  │                │   MariaDB       │ │    Redis      │                              │  │
│  │                │   Database      │ │    Cache      │                              │  │
│  │                └─────────────────┘ └────────────────┘                              │  │
│  │                                                                                            │  │
│  └───────────────────────────────────────────────────────────────────────────────────────┘  │
│                                         │                                                    │
│                                         v                                                    │
│                                  ┌─────────────────┐                                         │
│                                  │    Traefik     │                                         │
│                                  │  HTTPS + TLS  │                                         │
│                                  └────────┬────────┘                                         │
└────────────────────────────────────────────────┬───────────────────────────────────────────────────────┘
                                            │
                                            v
                               https://twitch-chatters.vignemail1.eu
```

- **Gateway** : API HTTP, authentification OAuth Twitch, gestion sessions
- **Twitch-API** : Wrapper API Twitch avec rate limiting
- **Worker** : Traitement asynchrone des jobs (captures, enrichissement)
- **Analysis** : API d'analyse et statistiques
- **MariaDB** : Base de données relationnelle (utilisateurs, sessions, captures)
- **Redis** : Cache distribué, sessions, rate limiting
- **Traefik** : Reverse proxy, terminaison TLS, load balancing

### Stack Technique

- **Backend** : Go 1.25
- **Base de données** : MariaDB 11.2
- **Cache** : Redis 7
- **Reverse Proxy** : Traefik v3.6
- **Containerisation** : Docker + Docker Compose
- **TLS** : Let's Encrypt (automatique)
- **Gestion d'environnement** : **mise** (outils + variables)
- **Gestion des secrets** : **fnox** (stockage sécurisé)

## 🚀 Quick Start

### Prérequis

- Docker 24+
- Docker Compose v2+
- Go 1.25+ (pour développement)
- [mise](https://mise.jdx.dev) (gestion environnement)
- Compte Twitch Developer (OAuth app)

### Installation

```bash
# 1. Installer mise (https://mise.jdx.dev/getting-started.html)
curl https://mise.run | sh

# 2. Cloner le repository
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# 3. Installer les outils (Go, Node, Python, fnox)
mise install

# 4. Activer mise dans votre shell
eval "$(mise activate bash)"  # ou zsh, fish
```

## 🌐 Configuration DNS

### Enregistrements DNS à Configurer

Avant de démarrer, configurez les enregistrements DNS suivants :

#### Production

| Domaine | Type | Valeur | Usage |
|---------|------|--------|-------|
| `twitch-chatters.vignemail1.eu` | A | `<IP_SERVEUR>` | Application principale |
| `traefik.vignemail1.eu` | A | `<IP_SERVEUR>` | Dashboard Traefik |
| `grafana.vignemail1.eu` | A | `<IP_SERVEUR>` | Monitoring Grafana |
| `prometheus.vignemail1.eu` | A | `<IP_SERVEUR>` | Métriques Prometheus |
| `alerts.vignemail1.eu` | A | `<IP_SERVEUR>` | Alertmanager |

#### Development

| Domaine | Type | Valeur | Usage |
|---------|------|--------|-------|
| `twitch-chatters-dev.vignemail1.eu` | A | `<IP_SERVEUR_DEV>` | Application dev |

#### Staging (Optionnel)

| Domaine | Type | Valeur | Usage |
|---------|------|--------|-------|
| `twitch-chatters-staging.vignemail1.eu` | A | `<IP_SERVEUR_STAGING>` | Application staging |

### Exemple de Configuration DNS (Cloudflare, OVH, etc.)

```dns
# Production
twitch-chatters     IN  A  51.178.95.123
traefik             IN  A  51.178.95.123
grafana             IN  A  51.178.95.123
prometheus          IN  A  51.178.95.123
alerts              IN  A  51.178.95.123

# Development (peut être le même serveur)
twitch-chatters-dev IN  A  51.178.95.123
```

### Vérification DNS

```bash
# Vérifier que les DNS sont propagés
dig +short twitch-chatters.vignemail1.eu
# 51.178.95.123

nslookup traefik.vignemail1.eu
# Server: 8.8.8.8
# Address: 8.8.8.8#53
# Name: traefik.vignemail1.eu
# Address: 51.178.95.123
```

**⚠️ Important** : Attendre que les DNS soient propagés (5-30 minutes) avant de lancer l'application pour que Let's Encrypt puisse générer les certificats TLS.

## 🌐 Environnements (Dev / Prod)

### Architecture mise + fnox

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  mise (https://mise.jdx.dev)                                                 │
│  │                                                                           │
│  ├── Gestion des outils (Go, Node, Python)                                  │
│  ├── Gestion des profils (development, staging, production)                │
│  ├── Variables d'environnement par profil                                  │
│  └── Tâches automatisées (build, up, logs, etc.)                          │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  fnox (https://fnox.jdx.dev)                                                 │
│  │                                                                           │
│  ├── Stockage sécurisé des secrets                                           │
│  ├── Secrets par environnement (dev, staging, prod)                         │
│  ├── Injection automatique dans l'environnement                             │
│  └── Pas de secrets en clair dans les fichiers                              │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**mise** gère l'environnement, **fnox** gère les secrets de manière sécurisée.

### Profils Disponibles

| Profil | Fichier | Usage | Monitoring | Domaine |
|--------|---------|-------|------------|----------|
| **development** | `.env.development` | Développement local | ❌ Désactivé | `twitch-chatters-dev.vignemail1.eu` |
| **staging** | `.env.staging` | Tests pré-prod | ✅ Activé | `twitch-chatters-staging.vignemail1.eu` |
| **production** | `.env.production` | Production | ✅ Activé | `twitch-chatters.vignemail1.eu` |

### Configuration avec fnox (Recommandé)

```bash
# 1. Définir les secrets pour development
fnox secret set development TWITCH_CLIENT_ID
# Prompt: Enter value for TWITCH_CLIENT_ID: ***

fnox secret set development TWITCH_CLIENT_SECRET
fnox secret set development MYSQL_ROOT_PASSWORD
fnox secret set development MYSQL_PASSWORD
fnox secret set development APP_SESSION_SECRET
fnox secret set development TRAEFIK_AUTH
fnox secret set development ACME_EMAIL

# 2. Répéter pour production (secrets DIFFÉRENTS!)
fnox secret set production TWITCH_CLIENT_ID
fnox secret set production TWITCH_CLIENT_SECRET
# etc.

# 3. Lister les secrets
fnox secrets list
# development:
#   - TWITCH_CLIENT_ID
#   - TWITCH_CLIENT_SECRET
#   - MYSQL_ROOT_PASSWORD
#   ...

# 4. Exporter vers .env (optionnel)
fnox secrets export development > .env.development
fnox secrets export production > .env.production
```

### Configuration Alternative (sans fnox)

```bash
# 1. Générer les secrets
mise run secrets:generate > secrets.txt
cat secrets.txt

# 2. Créer les fichiers .env
cp .env.example .env.development
cp .env.example .env.production

# 3. Éditer avec des secrets DIFFÉRENTS
vim .env.development
vim .env.production
```

### Utilisation des Profils

#### Mode Development (par défaut)

```bash
# Activer le profil development
export MISE_ENV=development
# ou
mise run env:dev

# Vérifier la configuration
mise run env-check
# 🌐 Environnement: development
# 🔗 Redirect URL: https://twitch-chatters-dev.vignemail1.eu/auth/callback
# 📊 Monitoring: false

# Démarrer
mise run up
# 🚀 Démarrage sans monitoring (development)
```

#### Mode Production

```bash
# Activer le profil production
export MISE_ENV=production
# ou
mise run env:prod

# Vérifier
mise run env-check
# 🌐 Environnement: production
# 🔗 Redirect URL: https://twitch-chatters.vignemail1.eu/auth/callback
# 📊 Monitoring: true

# Démarrer avec monitoring
mise run up
# 📊 Démarrage avec monitoring (production)
```

### Différences par Environnement

| Paramètre | Development | Production |
|-----------|-------------|------------|
| `APP_ENV` | `development` | `production` |
| `LOG_LEVEL` | `DEBUG` | `INFO` |
| Monitoring | ❌ Désactivé | ✅ Activé |
| Redis Port | `6379` (exposé) | Non exposé |
| MySQL Port | `3306` (exposé) | Non exposé |
| Rate Limit | `50 req/s` | `10 req/s` |
| Job Poll | `1s` | `2s` |
| Cache TTL | `30s` | `300s` |
| Redirect URL | `*-dev.*` | Production |

## 🔒 Sécurité

### Gestion des Secrets avec fnox

**✅ Avantages fnox** :
- ✅ Stockage sécurisé (chiffré localement)
- ✅ Pas de secrets en clair dans les fichiers
- ✅ Gestion par environnement (dev/staging/prod)
- ✅ Injection automatique dans l'environnement
- ✅ Partage sécurisé entre équipes

```bash
# Définir un secret (saisie sécurisée)
fnox secret set development MYSQL_ROOT_PASSWORD

# Lister les secrets (valeurs masquées)
fnox secrets list

# Utiliser les secrets
fnox run --env development mise run up

# Supprimer un secret
fnox secret rm development MYSQL_ROOT_PASSWORD
```

### Variables Requises

```bash
# Vérifier que toutes les variables sont définies
mise run env-check

# Générer des secrets forts
mise run secrets:generate

# Configurer fnox (aide)
mise run secrets:setup
```

### Sécurité Infrastructure

- ✅ TLS automatique via Let's Encrypt
- ✅ Redirection HTTP → HTTPS
- ✅ OAuth Twitch pour authentification
- ✅ Sessions sécurisées (Redis)
- ✅ Rate limiting distribué
- ✅ Mots de passe hashés (bcrypt)
- ✅ Base de données non exposée publiquement (prod)
- ✅ Secrets gérés par fnox (chiffrés)

## 🛠️ Tâches mise

### Tâches Principales

```bash
# Lister toutes les tâches
mise tasks

# Environnement
mise run env:dev        # Activer profil development
mise run env:prod       # Activer profil production
mise run env:staging    # Activer profil staging
mise run env-check      # Vérifier les variables

# Secrets (fnox)
mise run secrets:generate  # Générer secrets (fallback)
mise run secrets:setup     # Aide configuration fnox

# Build & Deploy
mise run install        # go mod download
mise run build          # docker-compose build
mise run build:nocache  # docker-compose build --no-cache
mise run up             # Démarrer (selon profil)
mise run up:dev         # Démarrer mode dev
mise run up:prod        # Démarrer mode prod
mise run down           # Arrêter
mise run down:volumes   # Arrêter + supprimer volumes
mise run restart        # Redémarrer

# Logs & Debug
mise run logs           # Tous les logs
mise run logs:gateway   # Logs gateway
mise run logs:worker    # Logs worker
mise run logs:db        # Logs database
mise run ps             # État des services

# Tests & Qualité
mise run test           # Tests unitaires
mise run test:coverage  # Tests + couverture
mise run lint           # Linter Go
mise run lint:fix       # Linter + fix auto

# Base de données
mise run db-backup      # Backup BDD
mise run db-restore <file.sql>  # Restaurer BDD
mise run db-console     # Console MariaDB
mise run redis-console  # Console Redis

# Maintenance
mise run clean          # Nettoyer fichiers temp
```

### Exemples d'Utilisation

```bash
# Workflow Development avec fnox
export MISE_ENV=development
fnox secret set development TWITCH_CLIENT_ID
fnox secret set development MYSQL_ROOT_PASSWORD
mise run env-check
fnox run --env development mise run up

# Workflow Production
export MISE_ENV=production
fnox secrets export production > .env.production
mise run env-check
mise run build:nocache
mise run up:prod

# Backup production
export MISE_ENV=production
mise run db-backup
# backup-production-20260209-143000.sql
```

## 📊 Ressources

### Configuration par Défaut (Development, 1 replica)

```
CPU  : ~3 vCPU (moyenne)
RAM  : ~4 GB
Disk : ~12 GB + données utilisateurs

Serveur recommandé : 4 vCPU, 8 GB RAM, 50 GB SSD
Coût estimé : ~12€/mois (Hetzner CPX31)
```

### Production avec Monitoring

```
CPU  : ~5 vCPU
RAM  : ~6 GB
Disk : ~21 GB + données utilisateurs

Serveur recommandé : 8 vCPU, 12 GB RAM, 80 GB SSD
Coût estimé : ~25€/mois (Hetzner CPX41)
```

## 📖 Documentation

### Guides Principaux

- [**SCALING.md**](docs/SCALING.md) : Scalabilité et optimisations de performance
- [**TRAEFIK.md**](docs/TRAEFIK.md) : Configuration Traefik et TLS
- [**MONITORING.md**](docs/MONITORING.md) : Stack de monitoring (Prometheus, Grafana, Loki)
- [**RESOURCES.md**](docs/RESOURCES.md) : Besoins en ressources et coûts
- [**DATABASE.md**](docs/DATABASE.md) : Structure BDD et migrations

### Architecture

- `cmd/gateway/` : Point d'entrée HTTP, OAuth, sessions
- `cmd/worker/` : Traitement asynchrone des jobs
- `cmd/analysis/` : API d'analyse et statistiques
- `cmd/twitch-api/` : Wrapper API Twitch avec rate limiting
- `internal/` : Packages partagés (redis, db, utils)
- `dev/` : Scripts de développement et schema SQL

## 🔧 Développement

### Setup Initial

```bash
# 1. Cloner et installer
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser
mise install
eval "$(mise activate bash)"

# 2. Configurer development avec fnox
export MISE_ENV=development
fnox secret set development TWITCH_CLIENT_ID
fnox secret set development TWITCH_CLIENT_SECRET
fnox secret set development MYSQL_ROOT_PASSWORD
fnox secret set development MYSQL_PASSWORD
fnox secret set development APP_SESSION_SECRET
fnox secret set development TRAEFIK_AUTH
fnox secret set development ACME_EMAIL

# 3. Vérifier et démarrer
mise run env-check
fnox run --env development mise run up:dev
```

### Tests

```bash
# Tests unitaires
mise run test

# Tests avec couverture
mise run test:coverage

# Linting
mise run lint
mise run lint:fix
```

## 📦 Base de Données

### Migrations

Le schéma est initialisé automatiquement au démarrage via `dev/schema.sql`.

Migrations manuelles dans `dev/migrations/` :

```bash
# Appliquer une migration
docker exec -i twitch-chatters-db mariadb -u root -p"$MYSQL_ROOT_PASSWORD" twitch_chatters < dev/migrations/001_limit_saved_sessions.sql
```

### Backup & Restore

```bash
# Backup automatique (utilise $APP_ENV)
mise run db-backup
# backup-development-20260209-143000.sql

# Restore
mise run db-restore backup-development-20260209-143000.sql
```

### Accès Direct

```bash
# Console MariaDB
mise run db-console

# Console Redis
mise run redis-console
```

## 📊 Monitoring (Production)

### Activation

```bash
# Le monitoring est activé automatiquement en profil production
export MISE_ENV=production
mise run up
# 📊 Démarrage avec monitoring (production)
```

### Services Inclus

- **Prometheus** : Métriques time-series
- **Grafana** : Dashboards et visualisation
- **Loki** : Agrégation logs
- **Alertmanager** : Gestion alertes
- **Exporters** : Node, cAdvisor, Redis, MySQL

### Accès

- **Grafana** : https://grafana.vignemail1.eu (admin/admin)
- **Prometheus** : https://prometheus.vignemail1.eu
- **Alertmanager** : https://alerts.vignemail1.eu

## 🔧 Maintenance

### Mise à Jour

```bash
# Pull derniers changements
git pull

# Rebuild et redémarrer
mise run build:nocache
mise run restart
```

### Nettoyage

```bash
# Arrêter services
mise run down

# Supprimer aussi les volumes (ATTENTION: perte de données)
mise run down:volumes

# Nettoyer images non utilisées
docker system prune -a

# Nettoyer fichiers temp Go
mise run clean
```

## 🚀 Performance

### Capacité Actuelle (Development, 1 replica)

- ✅ 100-500 utilisateurs actifs simultanés
- ✅ 1000-5000 captures/heure
- ✅ 10-50 requêtes HTTP/sec

### Production avec Replicas

```bash
# 2 gateway, 3 workers, 2 analysis
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

# Capacité
- ✅ 500-1000 utilisateurs actifs
- ✅ 5000-20000 captures/heure
- ✅ 50-200 requêtes HTTP/sec
```

## 📝 Licence

MIT License - Voir [LICENSE](LICENSE) pour plus de détails

## 👥 Auteur

**vignemail1**
- GitHub: [@vignemail1](https://github.com/vignemail1)
- Email: vignemail1@gmail.com

## 🚀 Roadmap

- [ ] Interface web frontend (React/Vue)
- [ ] Exports CSV/JSON des analyses
- [ ] Webhooks Discord/Slack
- [ ] API publique avec clés d'API
- [ ] Support multi-chaînes
- [ ] Détection des raids
- [ ] Analyse sentiment (IA)

---

**👍 Vous utilisez ce projet ?** N'hésitez pas à ⭐ star le repo !
