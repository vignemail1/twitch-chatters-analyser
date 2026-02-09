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
- **Gestion d'environnement** : mise / fnox

## 🚀 Quick Start

### Prérequis

- Docker 24+
- Docker Compose v2+
- Go 1.25+ (pour développement)
- [mise](https://mise.jdx.dev) ou [fnox](https://fnox.jdx.dev) (recommandé)
- Compte Twitch Developer (OAuth app)

### Installation avec mise/fnox (Recommandé)

```bash
# Installer mise (https://mise.jdx.dev/getting-started.html)
curl https://mise.run | sh

# Ou installer fnox (https://fnox.jdx.dev)
cargo install --locked fnox

# Cloner le repository
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# Activer direnv (optionnel mais recommandé)
direnv allow

# Installer les outils (Go, Docker, etc.)
mise install

# Générer les secrets pour .env
mise run env-generate > secrets.txt
cat secrets.txt

# Copier .env.example et y ajouter les valeurs
cp .env.example .env
vim .env

# Vérifier que toutes les variables sont définies
mise run env-check
```

### Installation Manuelle

```bash
# Cloner le repository
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# Copier la configuration
cp .env.example .env

# Générer les secrets
echo "MYSQL_ROOT_PASSWORD=$(openssl rand -base64 32)" >> .env
echo "MYSQL_PASSWORD=$(openssl rand -base64 32)" >> .env
echo "APP_SESSION_SECRET=$(openssl rand -base64 32)" >> .env

# Éditer .env et remplir les valeurs manquantes
vim .env
```

### Configuration .env

**⚠️ IMPORTANT** : Aucun secret n'a de valeur par défaut. Le fichier `.env` est **obligatoire**.

```bash
# Twitch OAuth (https://dev.twitch.tv/console/apps)
TWITCH_CLIENT_ID=votre_client_id
TWITCH_CLIENT_SECRET=votre_client_secret
TWITCH_REDIRECT_URL=https://twitch-chatters.vignemail1.eu/auth/callback

# Base de données (générer avec: openssl rand -base64 32)
MYSQL_ROOT_PASSWORD=votre_mot_de_passe_root_fort
MYSQL_DATABASE=twitch_chatters
MYSQL_USER=twitch
MYSQL_PASSWORD=votre_mot_de_passe_app_fort

# Session secret (générer avec: openssl rand -base64 32)
APP_SESSION_SECRET=votre_secret_fort

# Traefik
ACME_EMAIL=votre-email@example.com
# Générer avec: echo $(htpasswd -nB admin) | sed -e s/\$/\$\$/g
TRAEFIK_AUTH=admin:$$2y$$05$$...

# Environnement
APP_ENV=production
```

### DNS

Configurer les enregistrements DNS :

```dns
twitch-chatters.vignemail1.eu      A    <IP_SERVEUR>
twitch-chatters-dev.vignemail1.eu  A    <IP_SERVEUR>
traefik.vignemail1.eu              A    <IP_SERVEUR>
```

### Démarrage

```bash
# Avec mise
mise run up

# Ou manuellement
docker-compose up -d

# Vérifier les logs
mise run logs
# ou
docker-compose logs -f

# Vérifier l'état
docker-compose ps
```

### Accès

- **Application** : https://twitch-chatters.vignemail1.eu
- **Dashboard Traefik** : https://traefik.vignemail1.eu (admin/votre_mot_de_passe)

## 🔒 Sécurité

### Gestion des Secrets

**✅ Bonnes pratiques implémentées** :
- ❌ **Aucun** mot de passe ou secret en clair dans le code
- ✅ **Toutes** les valeurs sensibles dans `.env` (git ignored)
- ✅ Génération automatique des secrets forts
- ✅ Vérification des variables requises au démarrage
- ✅ Documentation complète dans `.env.example`

### Variables Requises

```bash
# Vérifier que toutes les variables sont définies
mise run env-check

# Générer des secrets forts automatiquement
mise run env-generate
```

### Sécurité Infrastructure

- ✅ TLS automatique via Let's Encrypt
- ✅ Redirection HTTP → HTTPS
- ✅ OAuth Twitch pour authentification
- ✅ Sessions sécurisées (Redis)
- ✅ Rate limiting distribué
- ✅ Mots de passe hashés (bcrypt)
- ✅ Base de données non exposée publiquement

## 🛠️ Gestion d'Environnement

### mise (Recommandé)

[mise](https://mise.jdx.dev) est un gestionnaire d'outils et de variables d'environnement.

```bash
# Configuration dans .mise.toml
[tools]
go = "1.25"
docker = "latest"

# Tâches disponibles
mise tasks

# Exécuter une tâche
mise run install    # Installer les dépendances
mise run build      # Builder les images
mise run up         # Démarrer les services
mise run down       # Arrêter les services
mise run logs       # Afficher les logs
mise run test       # Lancer les tests
mise run env-check  # Vérifier .env
```

### fnox (Alternative)

[fnox](https://fnox.jdx.dev) est compatible avec la configuration mise.

```bash
# Installer fnox
cargo install --locked fnox

# Utilisation identique à mise
fnox run up
fnox run logs
```

### direnv (Optionnel)

Pour charger automatiquement `.env` en entrant dans le répertoire :

```bash
# Installer direnv
brew install direnv  # macOS
sudo apt install direnv  # Ubuntu

# Ajouter dans ~/.bashrc ou ~/.zshrc
eval "$(direnv hook bash)"
# ou
eval "$(direnv hook zsh)"

# Autoriser le projet
cd twitch-chatters-analyser
direnv allow
```

## 📊 Ressources

### Configuration par Défaut (1 replica par service)

```
CPU  : ~3 vCPU (moyenne)
RAM  : ~4 GB
Disk : ~12 GB + données utilisateurs

Serveur recommandé : 4 vCPU, 8 GB RAM, 50 GB SSD
Coût estimé : ~12€/mois (Hetzner CPX31)
```

### Avec Monitoring (Optionnel)

```bash
# Démarrer avec monitoring
mise run up -- -f docker-compose.monitoring.yml
# ou
docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d

# Ressources supplémentaires
CPU  : +1.5 vCPU
RAM  : +2 GB
Disk : +9 GB

Serveur recommandé : 8 vCPU, 12 GB RAM, 80 GB SSD
```

### Scalabilité Horizontale

```bash
# Augmenter les replicas (en cas de charge)
docker-compose up -d --scale gateway=2 --scale worker=3 --scale analysis=2

# Ressources avec replicas
CPU  : ~6 vCPU
RAM  : ~7 GB

Serveur recommandé : 8 vCPU, 16 GB RAM, 80 GB SSD
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

### Mode Développement

```bash
# Démarrer en mode dev (1 replica, ports exposés)
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d

# Accès direct
curl http://localhost:8080  # Gateway
curl http://localhost:8083  # Analysis
```

### Build Local

```bash
# Builder les services
mise run build
# ou
docker-compose build

# Rebuild sans cache
docker-compose build --no-cache
```

### Tests

```bash
# Tests unitaires
mise run test
# ou
go test ./...

# Tests avec couverture
go test -cover ./...

# Linting
mise run lint
# ou
golangci-lint run
```

## 📦 Base de Données

### Migrations

Le schéma est initialisé automatiquement au démarrage via `dev/schema.sql`.

Migrations manuelles dans `dev/migrations/` :

```bash
# Appliquer une migration
docker exec -i twitch-chatters-db mariadb -u root -p twitch_chatters < dev/migrations/001_limit_saved_sessions.sql
```

### Backup

```bash
# Backup complet
docker exec twitch-chatters-db mariadb-dump -u root -p twitch_chatters > backup.sql

# Restauration
docker exec -i twitch-chatters-db mariadb -u root -p twitch_chatters < backup.sql
```

### Accès Direct

```bash
# Console MariaDB
docker exec -it twitch-chatters-db mariadb -u twitch -p

# Console Redis
docker exec -it twitch-chatters-redis redis-cli
```

## 📊 Monitoring (Optionnel)

### Services Inclus

- **Prometheus** : Métriques time-series
- **Grafana** : Dashboards et visualisation
- **Loki** : Agrégation logs
- **Alertmanager** : Gestion alertes
- **Exporters** : Node, cAdvisor, Redis, MySQL

### Accès Monitoring

- **Grafana** : https://grafana.vignemail1.eu (admin/admin)
- **Prometheus** : https://prometheus.vignemail1.eu
- **Alertmanager** : https://alerts.vignemail1.eu

## 🔧 Maintenance

### Mise à Jour

```bash
# Pull derniers changements
git pull

# Rebuild et redémarrer
mise run build
mise run up
```

### Nettoyage

```bash
# Arrêter et supprimer les containers
mise run down
# ou
docker-compose down

# Supprimer aussi les volumes (ATTENTION : perte de données)
docker-compose down -v

# Nettoyer images non utilisées
docker system prune -a
```

### Logs

```bash
# Tous les logs
mise run logs

# Logs d'un service
docker-compose logs -f gateway

# Filtrer les erreurs
docker-compose logs gateway | grep -i error
```

## 🚀 Performance

### Capacité Actuelle (1 replica)

- ✅ 100-500 utilisateurs actifs simultanés
- ✅ 1000-5000 captures/heure
- ✅ 10-50 requêtes HTTP/sec

### Avec Replicas (2 gateway, 3 workers, 2 analysis)

- ✅ 500-1000 utilisateurs actifs
- ✅ 5000-20000 captures/heure
- ✅ 50-200 requêtes HTTP/sec

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
