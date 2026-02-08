# Twitch Chatters Analyser

Twitch Chatters Analyser est un outil destiné aux **modérateurs Twitch** pour analyser les comptes présents dans le chat d'une chaîne et aider à déterminer si le nombre de spectateurs affiché est légitime ou fortement influencé par des bots.

L'application :

- s'authentifie via Twitch pour récupérer un **token user** avec les scopes nécessaires ;
- permet de **lister les chaînes** pour lesquelles l'utilisateur est modérateur ;
- capture les **chatters en temps réel** pour une chaîne donnée ;
- agrège ces comptes par **date de création** (jour/mois) afin de détecter des vagues de comptes créés en masse ;
- supporte plusieurs utilisateurs en parallèle, en isolant leurs sessions et données d'analyse.

Le projet est découpé en plusieurs **micro‑services en Go**, orchestrés via Docker Compose, avec MySQL comme base de données centrale.

---

## Objectifs

- Aider les modérateurs à détecter des **viewer bots** en observant :
  - les dates de création des comptes présents dans le chat ;
  - les concentrations anormales de comptes créés sur un ou quelques jours ;
  - les comptes qui **changent souvent de pseudo** (cas potentiellement suspects).
- Proposer une interface **mobile‑first**, utilisable rapidement pendant un live.
- Fournir un système **multi‑utilisateur** avec isolation des données par session d'analyse.

---

## Fonctionnalités principales

- **Authentification Twitch (OAuth2)**  
  - Connexion via Twitch.  
  - Récupération d'un token user avec les scopes :
    - `user:read:moderated_channels`
    - `moderator:read:chatters`
  - Les tokens sont stockés dans la session web et **supprimés** en fin de session ou lors de la déconnexion.

- **Sessions d'analyse**
  - Chaque utilisateur peut créer une **session d'analyse** identifiée par un UUID.
  - Une session regroupe une ou plusieurs captures de chatters pour une chaîne donnée.
  - Une session a une durée de vie limitée (expiration automatique) sauf si l'utilisateur la marque comme **sauvegardée**.

- **Liste des chaînes modérées**
  - Récupération de la liste des broadcasters pour lesquels l'utilisateur est modérateur.
  - Affichage sous forme de tableau (responsive), avec un bouton pour lancer une **capture de chatters**.

- **Capture de chatters**
  - Récupération de **tous les chatters** présents sur une chaîne (pagination Helix `getChatters`).
  - Enregistrement dans la base des :
    - chatters liés à une capture et à une session ;
    - informations de base des comptes Twitch (`id`, `login`, `display_name`, `created_at`, etc.).
  - Résumé affiché à l'utilisateur :
    - nombre total de chatters capturés ;
    - nombre de comptes encore inconnus ajoutés en base.

- **Analyse**
  - Pour chaque session/broadcaster :
    - nombre total de comptes référencés ;
    - **top 10 des jours de création** de comptes (date + nombre) ;
    - filtres sur les X derniers jours / semaines ;
    - export **CSV** et **JSON** des agrégations.
  - Utilisation d'**Apache ECharts** pour les graphiques (bar/line).

- **Renommage de comptes**
  - L'identifiant **unique** d'un compte est toujours `twitch_user_id` (ID Helix).
  - Les `login`/`display_name` peuvent changer :
    - chaque changement détecté est historisé dans une table dédiée avec la date de détection ;
    - un nombre élevé de renommages pour un compte peut être traité comme un signal suspect dans des analyses futures.

- **Traitement asynchrone et rate limiting**
  - Les captures ne font pas directement des dizaines d'appels Twitch depuis la requête HTTP.
  - À la place, le backend crée des **jobs** dans une file (MySQL), traités par un worker asynchrone.
  - Un service dédié gère les appels Helix et applique un **rate limiting global** pour rester sous les limites Twitch.

- **Traçabilité**
  - Chaque événement important est loggé :
    - authentification, déconnexion, expiration de session,
    - création/sauvegarde/chargement de session d'analyse,
    - demande de capture et fin de capture,
    - export CSV/JSON,
    - détection de renommage de compte.
  - Les logs sont à la fois :
    - en base (table d'audit),
    - en stdout pour centralisation via Docker/host.

---

## Architecture (résumé)

Le projet est découpé en plusieurs micro‑services Go :

- **gateway**  
  - HTTP server principal (server‑rendered)  
  - gère l'auth Twitch, les sessions web, les pages HTML, les exports.  

- **twitch-api**  
  - encapsule tous les appels à l'API Twitch Helix (OAuth, getChatters, users, moderated channels).  
  - impose le rate limiting global.  

- **worker**  
  - consomme les jobs depuis la base (captures, enrichissement des comptes).  
  - appelle `twitch-api` pour toutes les requêtes externes.  

- **analysis**  
  - fournit des endpoints internes pour les agrégations (top jours, exports, stats).  

Tous ces services partagent la même base MySQL.

La documentation détaillée de l'architecture et des flux se trouve dans [`dev/architecture.md`](dev/architecture.md).

---

## Démarrage rapide (à venir)

Le projet est encore en phase de conception. À terme, l'objectif est de pouvoir :

```bash
git clone https://github.com/vignemail1/twitch-chatters-analyser.git
cd twitch-chatters-analyser

# configuration (variables d'environnement ou fichier .env)
cp dev/example.env .env
# édition de .env (identifiants Twitch, DSN MySQL, secrets...)

# lancement en local
docker-compose up --build
```

---

## Statut du projet

- [x] Définition du besoin fonctionnel  
- [x] Choix technologique (Go, MySQL, Docker, ECharts)  
- [ ] Schéma SQL détaillé  
- [ ] Squelettes de services Go  
- [ ] Implémentation des flux OAuth + captures  
- [ ] Première version utilisable

---

## Contribuer

Le projet est actuellement développé pour répondre à un besoin précis de modération. Les contributions sont les bienvenues une fois la première version stabilisée :

- améliorations d'UX (filtres, tri, nouvelles visualisations) ;
- détection plus avancée de comportements suspects ;
- intégration avec d'autres outils de modération.

Les détails (guidelines, style Go, etc.) seront précisés dans [`dev/contributing.md`](dev/contributing.md) ultérieurement.
