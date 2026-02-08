# Twitch Chatters Analyser

🔍 Outil d'analyse des chatters Twitch pour détecter les viewer bots en analysant les dates de création de comptes et les patterns suspects.

## 🎯 Objectif

Cette application aide les modérateurs Twitch à identifier les **viewer bots** en capturant les utilisateurs présents dans le chat d'une chaîne et en analysant leurs données de profil.

### Indicateurs de bots

- 📅 **Dates de création groupées** : Des dizaines ou centaines de comptes créés le même jour
- ⏱️ **Comptes récents** : Créés dans les dernières semaines/mois
- 🔄 **Changements fréquents de noms** : Historique de renommages suspects
- 📊 **Pics anormaux** : Vagues de création concentrées dans le temps

## ✨ Fonctionnalités

### ✅ Implémentées

- ✅ **Authentification OAuth2 Twitch** - Connexion sécurisée avec scopes modérateur
- ✅ **Capture automatique des chatters** - Via API Twitch avec traitement asynchrone
- ✅ **Enrichissement des profils** - Récupération dates de création et métadonnées
- ✅ **Analyse statistique avancée** - Top 10 jours de création avec indicateurs de suspicion
- ✅ **Sessions sauvegardées** - Conservation historique des analyses
- ✅ **Export CSV/JSON** - Export complet avec filtrage
- ✅ **Filtrage multi-broadcaster** - Cases à cocher pour sélectionner les chaînes à analyser
- ✅ **Timezone navigateur** - Affichage des dates dans le fuseau horaire local
- ✅ **Interface moderne** - Dark theme optimisé

### 🚧 En développement

- 🔄 **Historique des changements de noms** - Détection des renommages suspects
- 🔄 **Détection automatique de patterns** - Score de suspicion et alertes
- 🔄 **Service rate-limited centralisé** - Protection contre les bans API Twitch

### 📋 Roadmap

- [ ] Graphiques interactifs (Chart.js)
- [ ] Comparaison entre captures
- [ ] Notifications Discord/Slack
- [ ] API REST publique
- [ ] Recherche et filtres avancés

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
     │ - Export CSV/JSON       │
     └──────┬─────────────┬────┘
            │              │
            │              │ HTTP
            │              │
     MySQL  │       ┌──────┼───────────┐
       +    │       │     Analysis     │   (Port 8083)
      Jobs  │       │ - Agrégations   │
            │       │ - Top N dates   │
            │       │ - Filtres       │
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

1. **Gateway** (`cmd/gateway`) - Interface web + authentification OAuth2 Twitch + exports
2. **Worker** (`cmd/worker`) - Traite les jobs asynchrones (fetch chatters, enrich users)
3. **Analysis** (`cmd/analysis`) - Calcule les statistiques, agrégations et filtres
4. **Twitch-API** (`cmd/twitch-api`) - *(En développement)* Proxy avec rate limiting centralisé

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

Ouvrez [http://localhost:8080](http://localhost:8080) dans votre navigateur.

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
4. Vous pouvez capturer plusieurs chaînes dans la même session

### Étape 3 : Analyser les résultats

1. Allez sur **"/analysis"** pour voir le résumé
2. **Filtrez par chaîne** (si plusieurs chaînes capturées) :
   - Cochez/décochez les chaînes à analyser
   - Les statistiques s'actualisent automatiquement
3. Consultez le **Top 10 des jours de création de comptes**
4. Identifiez les **pics suspects** :
   - 🔴 **CRITIQUE** (100+ comptes/jour)
   - 🟠 **SUSPECT** (50-99 comptes/jour)
   - 🔵 **À SURVEILLER** (30-49 comptes/jour)
   - 🟢 **NORMAL** (< 30 comptes/jour)

### Étape 4 : Exporter ou sauvegarder

- **Exporter CSV/JSON** : Bouton en haut de la page d'analyse
- **Sauvegarder la session** : Conserve l'historique pour consultation ultérieure
- **Purger la session** : Supprime toutes les captures (action irréversible)

## 📊 Que regarder dans les résultats ?

### ⚠️ Signaux d'alerte

| Indicateur | Valeur suspecte | Explication |
|------------|-----------------|-------------|
| Comptes/jour | 100+ | 🔴 Vague de bots quasi-certaine |
| Comptes/jour | 50-99 | 🟠 Très probablement des bots |
| Comptes/jour | 30-49 | 🔵 Potentiellement suspect |
| Date de création | < 3 mois | Comptes très récents |
| Concentration | 3-5 jours | Vague de bots groupée |

### ✅ Cas normaux

- Distribution étalée sur plusieurs années
- Pas de pic supérieur à 20-30 comptes/jour
- Majorité de comptes anciens (> 1 an)
- Pics isolés peuvent être des raids légitimes

### 💡 Conseils d'analyse

- **Contexte important** : Un raid, un événement spécial ou une collaboration peut créer des pics normaux
- **Combinez les indicateurs** : Ne vous fiez pas à un seul critère
- **Historique** : Comparez plusieurs captures pour détecter des patterns récurrents
- **Filtrage par chaîne** : Si vous streamez sur plusieurs chaînes, analysez-les séparément

## 🔧 Développement

### Structure du projet

```
.
├── cmd/
│   ├── gateway/      # Interface web + auth + exports
│   ├── worker/       # Traitement asynchrone
│   ├── analysis/     # Service d'analyse
│   └── twitch-api/   # (TODO) Proxy rate-limité
├── web/
│   ├── static/       # CSS, JS
│   │   ├── css/
│   │   └── js/
│   └── templates/    # Templates HTML Go
├── dev/
│   ├── architecture.md    # Documentation technique
│   ├── development.md     # Guide développeur
│   └── schema.sql         # Schéma MySQL
├── docker-compose.yml
├── .env.example
└── README.md
```

### Lancer en mode dev

```bash
# Rebuild après modification du code Go
docker-compose build
docker-compose up

# Voir les logs
docker-compose logs -f gateway
docker-compose logs -f worker
docker-compose logs -f analysis

# Accéder à la DB
docker-compose exec db mysql -u twitch -ptwitchpass twitch_chatters
```

### Rebuilder un service spécifique

```bash
docker-compose build gateway
docker-compose restart gateway
```

### Hot reload (développement local)

Pour développer sans Docker :

```bash
# Lancer uniquement MySQL
docker-compose up db

# Dans un autre terminal, lancer un service Go
cd cmd/gateway
go run .
```

## 💾 Base de données

### Tables principales

- `users` - Utilisateurs de l'app (modérateurs)
- `web_sessions` - Sessions web avec tokens Twitch
- `sessions` - Sessions d'analyse
- `captures` - Snapshots de chatters
- `capture_chatters` - Lien capture ↔ users
- `accounts` - Comptes Twitch dédupliqués
- `twitch_users` - Infos enrichies des comptes Twitch
- `twitch_user_names` - Historique des renommages
- `jobs` - File d'attente pour le worker

### Accéder à MySQL

```bash
# Via Docker
docker-compose exec db mysql -u twitch -ptwitchpass twitch_chatters

# Requêtes utiles
SELECT * FROM jobs ORDER BY id DESC LIMIT 10;
SELECT * FROM sessions WHERE status = 'active';
SELECT COUNT(*) FROM twitch_users WHERE created_at IS NOT NULL;
```

## 🚀 Production

### Sécurité

⚠️ **Avant de déployer en production** :

1. **Changez tous les mots de passe** dans `.env`
2. **Activez HTTPS** (requis pour OAuth2 Twitch)
3. **Mettez `Secure: true`** dans les cookies (main.go ligne ~250 et ~463)
4. **Limitez l'accès MySQL** (pas d'exposition publique)
5. **Sauvegardez régulièrement** la base de données
6. **Configurez un reverse proxy** (Traefik, Nginx, Caddy)

### Variables d'environnement importantes

```bash
APP_ENV=production
TWITCH_REDIRECT_URL=https://votre-domaine.com/auth/callback
MYSQL_ROOT_PASSWORD=mot-de-passe-fort-ici
DB_PASSWORD=autre-mot-de-passe-fort
APP_SESSION_SECRET=clé-secrète-aléatoire-longue-64-caracteres
```

### Reverse Proxy (Traefik, Nginx, Caddy)

Exposez uniquement le **gateway (port 8080)** publiquement. Les autres services (worker, analysis, db) doivent rester internes au réseau Docker.

Exemple Nginx :

```nginx
server {
    listen 443 ssl http2;
    server_name votre-domaine.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

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
docker-compose logs -f worker

# Vérifier la queue
docker-compose exec db mysql -u twitch -ptwitchpass -e "SELECT * FROM twitch_chatters.jobs ORDER BY id DESC LIMIT 10;"

# Redémarrer le worker
docker-compose restart worker
```

#### Les dates ne s'affichent pas dans mon fuseau horaire

→ Le JavaScript `timezone.js` se charge automatiquement. Vérifiez la console du navigateur pour d'éventuelles erreurs.

#### Export CSV vide

→ Attendez que le worker enrichisse les comptes. Cela peut prendre quelques minutes selon le nombre de viewers.

## 📝 Documentation technique

Pour plus de détails sur l'architecture et le développement :

- [Architecture et conception](dev/architecture.md)
- [Guide du développeur](dev/development.md)
- [Schéma de base de données](dev/schema.sql)

## 🤝 Contribution

Les contributions sont les bienvenues !

1. Fork le projet
2. Créez une branche (`git checkout -b feature/amazing-feature`)
3. Committez vos changements (`git commit -m 'feat: add amazing feature'`)
4. Push vers la branche (`git push origin feature/amazing-feature`)
5. Ouvrez une Pull Request

### Conventions de commit

- `feat:` Nouvelle fonctionnalité
- `fix:` Correction de bug
- `docs:` Documentation
- `refactor:` Refactoring
- `test:` Tests
- `chore:` Maintenance

## 📜 Licence

MIT License - Libre d'utilisation et modification

## 💬 Support

Problème ? Question ? Ouvrez une [issue](https://github.com/vignemail1/twitch-chatters-analyser/issues) !

---

🚀 **Happy bot hunting!** 🔍

Fait avec ❤️ pour la communauté Twitch
