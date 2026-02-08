# Twitch Chatters Analyser

🔍 Outil d'analyse des chatters Twitch pour détecter les viewer bots en analysant les dates de création de comptes et les patterns suspects.

## 🎯 Objectif

Cette application aide les modérateurs Twitch à identifier les **viewer bots** en capturant les utilisateurs présents dans le chat d'une chaîne et en analysant leurs données de profil.

### Indicateurs de bots

- 📅 **Dates de création groupées** : Des dizaines ou centaines de comptes créés le même jour
- ⏱️ **Comptes récents** : Créés dans les dernières semaines/mois
- 🔄 **Changements fréquents de noms** : Historique de renommages suspects
- 📊 **Pics anormaux** : Vagues de création concentrées dans le temps

## 🛠️ Architecture

L'application est composée de 4 microservices Go :

```
┌───────────────────────────────────┐
│          Utilisateur (Modérateur)       │
└───────────────┬────────────────────┘
                │
                │ HTTP
                │
     ┌──────────┼────────────┐
     │         Gateway          │   (Port 8080)
     │ - Auth Twitch           │
     │ - Sessions utilisateur  │
     │ - Interface Web         │
     └──────┬─────────────┬────┘
            │              │
            │              │ HTTP
            │              │
     MySQL  │       ┌──────┼───────────┐
       +    │       │     Analysis     │   (Port 8083)
      Jobs  │       │ - Aggrégations  │
            │       │ - Top N dates   │
            │       └──────┬───────────┘
            │              │
     ┌──────┼───────       │ MySQL
     │     Worker       │       │
     │ - Fetch chatters│       │
     │ - Enrich users  │       │
     │ - Job queue     │       │
     └──────────┬───────       │
                │              │
                │ Twitch API   │
                │              │
                └────────────────┘
```

### Services

1. **Gateway** (`cmd/gateway`) - Interface web + authentification OAuth2 Twitch
2. **Worker** (`cmd/worker`) - Traite les jobs asynchrones (fetch chatters, enrich users)
3. **Analysis** (`cmd/analysis`) - Calcule les statistiques et aggrégations
4. **Twitch-API** (`cmd/twitch-api`) - (Optionnel) Proxy avec rate limiting centralisé

## ⚡ Installation rapide

### Prérequis

- Docker & Docker Compose
- Application Twitch (créée sur [dev.twitch.tv](https://dev.twitch.tv/console/apps))
- Être modérateur sur au moins une chaîne Twitch

### 1. Cloner le projet

```bash
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser
```

### 2. Configuration

Créez votre fichier `.env` :

```bash
cp .env.example .env
```

Éditez `.env` et remplissez **obligatoirement** :

```bash
# Obtenez ces valeurs sur https://dev.twitch.tv/console/apps
TWITCH_CLIENT_ID=votre_client_id
TWITCH_CLIENT_SECRET=votre_client_secret
TWITCH_REDIRECT_URL=http://localhost:8080/auth/callback

# Sécurisé pour la production
APP_SESSION_SECRET=changez-moi-en-production
```

### 3. Lancer l'application

```bash
docker-compose up -d
```

L'initialisation prend ~30 secondes (création de la DB).

### 4. Accéder à l'interface

Ouvrez http://localhost:8080 dans votre navigateur.

## 📚 Utilisation

### Étape 1 : Connexion

1. Cliquez sur **"Se connecter avec Twitch"**
2. Autorisez les permissions demandées :
   - `user:read:moderated_channels` - Lister vos chaînes modérées
   - `moderator:read:chatters` - Lire les chatters du salon

### Étape 2 : Capturer les chatters

1. Allez sur **"/channels"** pour voir vos chaînes modérées
2. Cliquez sur **"Capturer les chatters"** pour la chaîne à analyser
3. Le worker traite la capture en arrière-plan (quelques secondes à minutes selon le nombre de viewers)

### Étape 3 : Analyser les résultats

1. Allez sur **"/analysis"** pour voir le résumé
2. Consultez le **Top 10 des jours de création de comptes**
3. Identifiez les **pics suspects** (100+ comptes le même jour = suspect)

## 📊 Que regarder dans les résultats ?

### ⚠️ Signaux d'alerte

| Indicateur | Valeur suspecte | Explication |
|------------|-----------------|-------------|
| Comptes/jour | 50+ | Pic anormal de créations |
| Date de création | < 3 mois | Comptes très récents |
| Concentration | 3-5 jours | Vague de bots groupée |

### ✅ Cas normaux

- Distribution étalée sur plusieurs années
- Pas de pic supérieur à 20-30 comptes/jour
- Majorité de comptes anciens (> 1 an)

## 🔧 Développement

### Structure du projet

```
.
├── cmd/
│   ├── gateway/      # Interface web
│   ├── worker/       # Traitement asynchrone
│   ├── analysis/     # Service d'analyse
│   └── twitch-api/   # (TODO) Proxy rate-limité
├── web/
│   ├── static/       # CSS, JS
│   └── templates/    # Templates HTML Go
├── dev/
│   └── schema.sql    # Schéma MySQL
├── docker-compose.yml
└── .env.example
```

### Lancer en mode dev

```bash
# Rebuild après modification du code Go
docker-compose build
docker-compose up

# Voir les logs
docker-compose logs -f gateway
docker-compose logs -f worker

# Accéder à la DB
docker-compose exec db mysql -u twitch -ptwitchpass twitch_chatters
```

### Rebuilder un service spécifique

```bash
docker-compose build gateway
docker-compose restart gateway
```

## 💾 Base de données

### Tables principales

- `users` - Utilisateurs de l'app (modérateurs)
- `web_sessions` - Sessions web avec tokens Twitch
- `sessions` - Sessions d'analyse
- `captures` - Snapshots de chatters
- `capture_chatters` - Lien capture ↔ users
- `twitch_users` - Infos enrichies des comptes Twitch
- `twitch_user_names` - Historique des renommages
- `jobs` - File d'attente pour le worker

### Accéder à MySQL

```bash
docker-compose exec db mysql -u root -prootpass twitch_chatters
```

## 🚀 Production

### Sécurité

⚠️ **Avant de déployer en production** :

1. **Changez tous les mots de passe** dans `.env`
2. **Activez HTTPS** (requis pour OAuth2 Twitch)
3. **Mettez `Secure: true`** dans les cookies (main.go ligne ~250 et ~463)
4. **Limitez l'accès MySQL** (pas d'exposition publique)
5. **Sauvegardez régulièrement** la base de données

### Variables d'environnement importantes

```bash
APP_ENV=production
TWITCH_REDIRECT_URL=https://votre-domaine.com/auth/callback
MYSQL_ROOT_PASSWORD=mot-de-passe-fort-ici
APP_SESSION_SECRET=clé-secrète-aléatoire-longue
```

### Reverse Proxy (Traefik, Nginx, Caddy)

Exposez uniquement le **gateway (port 8080)** publiquement. Les autres services (worker, analysis, db) doivent rester internes au réseau Docker.

## 🐛 Débogage

### Problèmes courants

#### "Twitch auth not configured"

→ Vérifiez que `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET` et `TWITCH_REDIRECT_URL` sont bien définis dans `.env`

#### "failed to load channels" / 403 Forbidden

→ Vérifiez que vous êtes bien **modérateur** sur au moins une chaîne et que le scope `user:read:moderated_channels` est autorisé

#### "no active analysis session" sur /analysis

→ Capturez d'abord des chatters depuis `/channels` avant d'aller sur `/analysis`

#### Le worker ne traite pas les jobs

```bash
# Vérifier les logs
docker-compose logs worker

# Vérifier la queue
docker-compose exec db mysql -u twitch -ptwitchpass -e "SELECT * FROM twitch_chatters.jobs ORDER BY id DESC LIMIT 10;"
```

## 📝 TODO / Améliorations futures

- [ ] Service `twitch-api` avec rate limiting centralisé
- [ ] Historique des changements de noms (table `twitch_user_names`)
- [ ] Recherche/filtres avancés sur les résultats
- [ ] Export CSV/JSON des résultats
- [ ] Graphiques interactifs (Chart.js)
- [ ] Notifications Discord/Slack des résultats
- [ ] API REST publique pour intégrations externes
- [ ] Authentification multi-facteurs (2FA)
- [ ] Comparaison entre plusieurs captures
- [ ] Détection automatique de patterns suspects

## 📜 Licence

MIT License - Libre d'utilisation

## 💬 Support

Problème ? Ouvrez une [issue](https://github.com/vignemail1/twitch-chatters-analyser/issues) !

---

🚀 **Happy bot hunting!** 🔍
