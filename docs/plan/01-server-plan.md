# Partie A — Serveur (repo magpiemail-server)

## Contexte pour l'agent

Ce repo contient le serveur MagpieMail : API publique, synchronisation, règles, hooks, IA, stockage. Il ne contient aucune interface graphique ; le client vit dans un autre repo et ne connaît le serveur que par `api/openapi.yaml`. Lire `docs/plan/00-overview.md` (la vue d'ensemble) avant la première phase.

## Stack

| Domaine | Choix |
| --- | --- |
| Langage | Go, dernière version stable |
| HTTP | `net/http` + chi, serveur généré depuis le contrat (oapi-codegen, mode strict) |
| Temps réel | WebSocket (coder/websocket) |
| Base | PostgreSQL, pgx, sqlc, migrations goose |
| Tâches de fond | River |
| Mail | emersion/go-imap v2, emersion/go-smtp, emersion/go-message, golang.org/x/oauth2 |
| Sécurité | Argon2id, TOTP, WebAuthn (go-webauthn), OIDC (go-oidc), bluemonday (HTML), gopenpgp v3 (PGP) |
| Configuration | Variables d'environnement + fichier YAML (koanf) |
| Observabilité | `log/slog` en JSON ; métriques Prometheus et traces OpenTelemetry en option |
| Tests | `testing` + testify, Testcontainers, GreenMail ou Dovecot pour IMAP/SMTP, mock-oauth2-server, fuzzing natif Go |
| Qualité | golangci-lint, gofumpt, govulncheck, gitleaks |
| Livraison | Image Docker multi-étapes (base distroless), docker compose avec profils pour les conteneurs optionnels |

## Arborescence cible

```text
magpiemail-server/
├── api/openapi.yaml          # the contract (source of truth)
├── cmd/magpie/               # single binary: serve, worker, migrate, admin
├── internal/
│   ├── domain/               # entities and business rules, no I/O
│   ├── service/              # use cases
│   ├── store/                # postgres (sqlc) and blob storage
│   ├── transport/http/       # generated server + handlers
│   ├── transport/ws/         # real-time events
│   ├── mail/                 # imap, smtp, mime, provider adapters
│   ├── sync/  rules/  hooks/  ai/  search/  documents/  calendar/
│   ├── egress/               # the ONLY way out to the network
│   └── security/             # crypto, auth, policies
├── migrations/
├── deploy/                   # docker-compose.yml, sidecar configs
├── sdks/python/  sdks/rust/
├── test/                     # e2e, fixtures, mail corpus
└── docs/                     # plan/, adr/, guides/, PROGRESS.md
```

## CLAUDE.md à placer à la racine

```markdown
# CLAUDE.md — magpiemail-server

## Project
MagpieMail server: a self-hosted, privacy-first mail aggregation server
(multi-account, multi-user). All business logic lives here. Clients live in a
separate repo and only know this server through `api/openapi.yaml`.

## Language rules
- Always talk to the user in French.
- Everything committed is in English: code, identifiers, comments, commit
  messages, PR descriptions, docs, ADRs, log and error messages.
- Exception: `docs/plan/` is the user's plan, written in French.

## How to work
- Read `docs/plan/` first. Work on ONE phase at a time, as asked (e.g. "S5").
- Check in `docs/PROGRESS.md` that the phase's dependencies are done.
  If not, stop and tell the user.
- Contract first: update and lint `api/openapi.yaml` before implementing.
- TDD: failing test first, then minimal code, then refactor.
- Never tick a validation criterion without proof (test name, command output).
- Record non-trivial decisions as ADRs in `docs/adr/`.
- End of phase: run `task check`, update `docs/PROGRESS.md` and
  `CHANGELOG.md`, summarize in French, then stop for review.
- If the plan is ambiguous or looks wrong, ask instead of guessing.

## Commands
- `task check`            format, lint, unit tests
- `task test:integration` integration tests (Docker required)
- `task gen`              regenerate code from OpenAPI and SQL
- `task dev`              run the full stack with docker compose

## Architecture rules
- `internal/domain` has no I/O dependencies.
- All outbound network traffic goes through `internal/egress`.
- Never log message content, tokens or passwords.
- User-supplied regexes: Go `regexp` (RE2) only, with size limits.
- Secrets are encrypted at rest with the server master key.

## Conventions
Conventional Commits; branches `phase/SNN-short-name`; gofumpt and
golangci-lint clean; UUIDv7 ids; UTC timestamps; RFC 9457 errors.
```

## Phases

Vingt phases, S0 à S19, en cinq jalons. L'ordre respecte les dépendances : ne pas sauter une phase sans l'avoir notée dans `docs/PROGRESS.md`.

## Jalon 1 — Socle (S0 à S4)

À la fin du jalon, un serveur sécurisé accueille plusieurs users, applique les politiques de l'admin et expose un contrat d'API publié. Aucun mail n'est encore traité.

### S0 — Fondations du repo

**Objectif** : un repo vide mais complet, où `task check` et la CI passent et où `docker compose up` démarre un serveur qui répond.

**Dépend de** : rien.

**Tâches**

- Initialiser le module Go, l'arborescence cible, le Taskfile, golangci-lint et gofumpt.
- Créer le binaire `magpie` avec les sous-commandes `serve`, `worker`, `migrate`, `admin` (vides pour l'instant).
- Charger la configuration (variables d'environnement + YAML), la valider au démarrage avec des erreurs claires ; fournir un `config.example.yaml` commenté.
- Logs JSON structurés avec identifiant de requête ; endpoints `/healthz` (processus vivant) et `/readyz` (base joignable).
- Dockerfile multi-étapes (distroless, utilisateur non root) et `deploy/docker-compose.yml` avec PostgreSQL.
- CI : formatage, lint, tests unitaires et d'intégration, govulncheck, gitleaks, construction de l'image.
- ADR reprenant chaque décision de la vue d'ensemble ; `docs/PROGRESS.md`, `CHANGELOG.md`, README.

**Livrables** : repo initialisé, image Docker, premiers ADR.

**Critères de validation**

- [ ] `task check` passe en local et en CI.
- [ ] Après `docker compose up`, `/readyz` répond 200 en moins de 10 secondes.
- [ ] Une configuration invalide fait échouer le démarrage avec un message qui nomme la clé fautive (test).
- [ ] L'image tourne sans root et pèse moins de 50 Mo.

### S1 — Contrat d'API et squelette HTTP

**Objectif** : le contrat OpenAPI existe, est vérifié en CI, et toute requête traverse une chaîne commune (erreurs, limites, sécurité) prête à recevoir les domaines.

**Dépend de** : S0.

**Tâches**

- Écrire `api/openapi.yaml` : schémas de sécurité (bearer pour les clients natifs, cookie pour le web), schémas communs (erreur RFC 9457, pagination par curseur, enveloppe d'événement) et `GET /api/v1/info` (version, fonctions activées sur l'instance).
- Linter le contrat (Redocly ou Spectral) avec des règles maison : chaque opération a un `operationId`, un tag, des exemples et ses réponses d'erreur.
- Générer le serveur (oapi-codegen strict) via `task gen` ; la CI échoue si le code généré n'est pas à jour.
- Middlewares : identifiant de requête, logs d'accès sans données personnelles, reprise sur panique, taille de corps limitée, limitation de débit par IP et par user, CORS configurable, en-têtes de sécurité.
- En test, valider chaque réponse contre le contrat : une réponse non conforme fait échouer le test.
- Publier le contrat : documentation HTML générée, tag `api-v0.1.0`, script de publication réutilisable.

**Livrables** : contrat v0.1.0, serveur généré, middlewares, documentation d'API.

**Critères de validation**

- [ ] Le lint du contrat passe sans avertissement.
- [ ] Un test vérifie que chaque route servie existe dans le contrat.
- [ ] Les réponses 400, 401, 404, 413, 429 et 500 sont toutes en `application/problem+json` (tests).
- [ ] Le dépassement de débit renvoie 429 avec un en-tête `Retry-After`.

### S2 — Modèle de données et stockage

**Objectif** : toutes les données du MVP ont un schéma migrable, les fichiers ont un stockage dédupliqué, et les secrets sont chiffrés au repos ; le tout testé sur un vrai PostgreSQL.

**Dépend de** : S0.

**Tâches**

- Schéma initial : users, sessions, credentials (passkeys, TOTP), policies, providers, accounts, identities, sync\_policies, mailboxes, messages, message\_locations (message ↔ mailbox + UID), threads, tags, message\_tags, global\_folders, attachments, blobs, drafts, outbox, audit\_log.
- Clés UUIDv7, horodatages UTC, suppression douce là où une corbeille s'applique, index pensés pour les listes (account, mailbox, date décroissante).
- Requêtes typées avec sqlc, un repository par agrégat, transactions explicites.
- Stockage de blobs derrière une interface (disque local rangé par préfixe d'empreinte, S3-compatible) : écriture atomique, déduplication SHA-256, compteur de références, purge des orphelins.
- Chiffrement des secrets par enveloppe : clé maître fournie par variable ou secret Docker, une clé de données par user, commande `magpie admin rotate-key`.
- Chiffrement optionnel des blobs avec le même mécanisme.

**Livrables** : migrations, repositories, stockage de blobs, module de chiffrement.

**Critères de validation**

- [ ] Toutes les migrations montent et redescendent sur une base vide (test d'intégration).
- [ ] Les repositories sont couverts à plus de 80 % par des tests sur PostgreSQL réel.
- [ ] Écrire deux fois le même fichier ne crée qu'un blob ; un blob orphelin disparaît à la purge.
- [ ] Aucun secret n'est lisible en clair dans les colonnes brutes (test).
- [ ] La rotation de la clé maître rechiffre tout sans perte (test).

### S3 — Authentification et multi-utilisateur

**Objectif** : plusieurs users se connectent de façon sûre (mot de passe + second facteur, ou passkey), et chaque requête est authentifiée, autorisée et tracée.

**Dépend de** : S1, S2.

**Tâches**

- Créer le premier admin en ligne de commande (`magpie admin create-user --admin`), jamais via une page ouverte.
- Mots de passe en Argon2id, longueur minimale, refus des mots de passe courants (liste embarquée, aucun appel externe).
- Second facteur TOTP avec codes de secours ; passkeys WebAuthn, en second facteur ou en connexion sans mot de passe.
- Sessions : jeton d'accès de 15 minutes + jeton de rafraîchissement à usage unique, avec rotation et détection de réutilisation (toute la famille est révoquée). Mode cookie `HttpOnly; Secure; SameSite=Strict` pour le web, mode bearer pour les clients natifs.
- Gestion des appareils : liste des sessions, révocation, déconnexion partout.
- Jetons d'API personnels avec portées (`messages:read`, `rules:write`…) et expiration, pour les SDK et les hooks.
- SSO OIDC optionnel (Authelia, Authentik, Keycloak).
- Anti force brute : délai croissant par compte et par IP, verrouillage temporaire, notification à l'user.
- Rôles admin et user + squelette des capabilities, vérifiés dans la couche service et pas seulement dans les handlers.
- Journal d'audit : connexions, échecs, changements de sécurité, actions admin.

**Livrables** : tag `auth` du contrat, implémentation, guide « Account security ».

**Critères de validation**

- [ ] Tests : mot de passe faux, code TOTP rejoué, jeton expiré, jeton de rafraîchissement réutilisé (famille révoquée), passkey d'un autre user.
- [ ] Un user ne peut lire aucune ressource d'un autre user : tests d'accès croisé générés depuis le contrat, sur chaque endpoint.
- [ ] Après 10 échecs, le compte est verrouillé temporairement et l'événement figure au journal d'audit.
- [ ] Les exigences « authentification » et « sessions » de l'OWASP ASVS niveau 2 sont revues dans `docs/security/asvs.md`.

### S4 — Administration et politiques

**Objectif** : l'admin décide qui peut faire quoi (providers, IA, hooks, quotas), et le serveur applique chaque refus.

**Dépend de** : S3.

**Tâches**

- Gestion des users : création, invitation par lien à usage unique, désactivation, réinitialisation du second facteur.
- Catalogue de providers en YAML, modifiable par API : intégrés (Gmail, Outlook, Proton via Bridge) et custom. Champs : hôtes et ports IMAP/SMTP, TLS, méthodes d'auth, paramètres OAuth, particularités (labels Gmail…).
- Politiques globales et par user : providers autorisés, droit d'ajouter un provider custom, droit de synchroniser les actions, IA autorisée et providers IA permis, droit d'ajouter son provider IA, hooks accessibles, droit de créer ses hooks, quotas (stockage, appels IA par jour).
- Un service unique `can(user, capability, resource)` appelé par tous les services.
- `GET /api/v1/me/capabilities`, pour que le client masque ce qui est interdit.
- Paramètres d'instance : nom, inscriptions ouvertes ou non, durée des sessions, URL publique.

**Livrables** : tag `admin` du contrat, service de politique, tests.

**Critères de validation**

- [ ] Chaque capability a un test « autorisé » et un test « refusé » au niveau service.
- [ ] Retirer un provider du catalogue suspend la synchro des accounts concernés et prévient les users, sans perte de données.
- [ ] Toute action admin figure au journal d'audit.
- [ ] Un quota dépassé renvoie une erreur problem+json avec un type dédié.

## Jalon 2 — MVP mail (S5 à S11)

À la fin du jalon, le serveur synchronise plusieurs accounts en arrière-plan, sert des mails nettoyés et sans traqueurs, permet de ranger, chercher, écrire et envoyer, et pousse chaque changement aux clients en temps réel.

### S5 — Accounts et providers

**Objectif** : un user connecte ses comptes Gmail, Outlook, Proton et custom, sans mot de passe stocké quand OAuth existe, et le serveur vérifie que la connexion marche.

**Dépend de** : S4.

**Tâches**

- Interface `ProviderAdapter` (connexion, capacités, particularités) : adaptateur IMAP/SMTP générique, spécialisé pour Gmail (labels, identifiant de thread), Outlook et Proton Bridge.
- OAuth 2.0 avec PKCE et paramètre `state`, piloté par le serveur : le client ouvre l'URL d'autorisation, le retour arrive sur le serveur. Rafraîchissement automatique des jetons, XOAUTH2 sur IMAP et SMTP.
- Identifiants classiques ou mot de passe d'application pour les providers custom, chiffrés au repos.
- Proton : service `proton-bridge` dans un profil docker compose.
- Test de connexion qui renvoie les capacités annoncées (IDLE, CONDSTORE, QRESYNC, MOVE…) et des erreurs exploitables.
- Identities : adresse principale, alias (lus chez le provider quand c'est possible, sinon saisis), nom affiché, signature.
- Sync policy par account : répercuter lu / non lu, drapeaux, déplacements, suppressions, tags ; garder localement les mails supprimés chez le provider ; profondeur d'historique à importer.
- Guides : application OAuth Google (et ses limites hors vérification), application Microsoft Entra pour Outlook, Proton Bridge.

**Livrables** : tag `accounts` du contrat, adaptateurs, trois guides.

**Critères de validation**

- [ ] Ajout d'un account IMAP custom de bout en bout contre un serveur IMAP en conteneur.
- [ ] Flux OAuth complet testé contre mock-oauth2-server, y compris jeton rafraîchi et jeton révoqué (account marqué « à reconnecter »).
- [ ] Mauvais mot de passe, certificat invalide et port fermé donnent trois erreurs distinctes et lisibles.
- [ ] Aucun identifiant en clair en base ni dans les logs (test).
- [ ] Test manuel documenté avec un vrai compte Gmail et un vrai compte Outlook.

### S6 — Moteur de synchronisation

**Objectif** : les mails de tous les accounts arrivent sur le serveur en arrière-plan, vite et sans perte, et les actions faites dans MagpieMail repartent chez le provider selon la sync policy.

**Dépend de** : S5.

**Tâches**

- Machine à états par account (connexion, synchro initiale, veille, attente avant réessai, à reconnecter, désactivé), exécutée par les workers River : un worker peut tomber sans perte.
- Synchro initiale : liste des mailboxes, en-têtes d'abord pour un affichage rapide, puis corps et pièces jointes par lots, du plus récent au plus ancien.
- Synchro incrémentale : UIDVALIDITY, CONDSTORE / QRESYNC quand disponibles, IDLE pour le temps réel, sinon interrogation périodique réglable.
- Mail brut (.eml) en blob ; analyse MIME (texte, HTML, pièces jointes, images inline `cid:`, encodages exotiques) ; lecture des en-têtes SPF / DKIM / DMARC.
- Gmail : un mail présent sous plusieurs labels n'est stocké qu'une fois.
- File d'actions sortantes (lu, drapeau, déplacement, suppression, tag) : réessais, idempotence, règle de conflit documentée (le provider fait foi par défaut).
- Détection des suppressions et déplacements faits chez le provider.
- Événements internes (`message.received`, `message.updated`…) que consommeront règles, notifications et IA.
- Limites : connexions simultanées par provider, quotas Gmail et Outlook respectés, réessais à délai croissant.

**Livrables** : package `sync`, workers, métriques de synchro.

**Critères de validation**

- [ ] Scénarios testés contre un vrai serveur IMAP en conteneur : nouveau mail, suppression et déplacement chez le provider, drapeau changé des deux côtés, UIDVALIDITY réinitialisé, coupure réseau en pleine synchro.
- [ ] Avec IDLE, un nouveau mail apparaît sur le serveur en moins de 10 secondes.
- [ ] Une boîte de 10 000 mails finit sa synchro initiale en moins de 15 minutes sur la machine de dev (mesure consignée).
- [ ] Tuer un worker en pleine synchro ne crée ni doublon ni perte (test).
- [ ] Une action désactivée dans la sync policy n'envoie aucune commande au provider (test).

### S7 — Lecture, rendu sûr et anti-pistage

**Objectif** : le client liste et lit tous les mails, vue globale comprise, sans jamais recevoir de contenu exécutable ni devoir contacter Internet.

**Dépend de** : S6.

**Tâches**

- Endpoints : mailboxes, listes de messages (par account, par mailbox, vues « All » et « Unified Inbox »), détail, thread, pièces jointes en streaming, mail brut.
- Threads par l'algorithme JWZ (Message-ID, In-Reply-To, References), avec l'identifiant Gmail quand il existe ; regroupement réglable par user (activé, désactivé, repli sur le sujet).
- Nettoyage HTML côté serveur (bluemonday, liste blanche stricte) : ni script, ni formulaire, ni iframe, ni CSS dangereux.
- Anti-pistage : retrait des pixels espions (images 1×1, domaines connus d'une liste embarquée et remplaçable), retrait optionnel des paramètres `utm_*` ; nombre de traqueurs retirés renvoyé au client.
- Proxy de contenu distant : URL d'images réécrites vers `/api/v1/proxy/…`, signées et à durée limitée ; récupération via le module egress, avec proxy amont optionnel (Gluetun). Bloqué par défaut, autorisable par mail, par expéditeur ou globalement.
- Texte brut : rendu propre (format=flowed, citations).
- Indicateurs de sécurité : SPF / DKIM / DMARC, alerte sur nom affiché trompeur ou domaine ressemblant.
- Guide « Route remote content through a VPN with Gluetun ».

**Livrables** : tags `mailboxes` et `messages`, nettoyeur HTML, proxy, guide Gluetun.

**Critères de validation**

- [ ] Un corpus de mails piégés (XSS, CSS malveillant, iframes, liens `javascript:`) ressort entièrement neutralisé.
- [ ] Un corpus de vraies newsletters reste lisible et perd ses pixels espions.
- [ ] Le HTML servi ne contient aucune URL externe chargée automatiquement (test).
- [ ] Une URL de proxy modifiée ou expirée est refusée.
- [ ] Avec Gluetun configuré, les requêtes du proxy sortent par le VPN (test manuel documenté).

### S8 — Organisation : dossiers globaux, tags, actions

**Objectif** : un user range, étiquette et traite les mails de tous ses accounts au même endroit, et peut annuler chaque action.

**Dépend de** : S7.

**Tâches**

- Global folders : arborescence, création, renommage, déplacement, suppression ; ils acceptent des messages de n'importe quel account.
- Tags : nom, couleur, filtre ; répercussion optionnelle chez le provider (mots-clés IMAP, labels Gmail) selon la sync policy.
- Actions unitaires et en lot : lu / non lu, drapeau, archiver, déplacer, tagger, supprimer, spam / non-spam.
- Annulation : chaque action en lot renvoie un identifiant d'annulation valable quelques secondes.
- Corbeille et rétention : suppression réversible, purge planifiée, durée réglable par user.
- Désinscription en un clic (en-tête List-Unsubscribe, RFC 8058) via le module egress.

**Livrables** : tags `folders` et `tags` du contrat, actions, tâche de purge.

**Critères de validation**

- [ ] Une action sur 1 000 messages répond en moins de 2 secondes ; la répercussion chez le provider part en tâche de fond.
- [ ] L'annulation rétablit l'état exact d'avant, sur le serveur et chez le provider (test).
- [ ] Un tag répercuté en label apparaît dans Gmail (test manuel documenté).
- [ ] La purge ne supprime rien avant la fin de la rétention (test).

### S9 — Composition, brouillons et envoi

**Objectif** : un user écrit, répond et envoie depuis la bonne identity, programme ou annule un envoi, sans jamais perdre un brouillon.

**Dépend de** : S7.

**Tâches**

- Brouillons : création, sauvegarde automatique par mise à jour partielle, synchro optionnelle vers le dossier Drafts du provider.
- Pièces jointes : envoi par morceaux reprenable, limites de taille par provider.
- Répondre, répondre à tous, transférer : citation, en-têtes `In-Reply-To` et `References` corrects.
- Expéditeur par défaut : l'identity qui a reçu le mail (To, Cc, Delivered-To, X-Original-To), sinon celle par défaut de l'account.
- Construction MIME : texte + HTML (multipart/alternative), images inline, noms et sujets encodés correctement.
- Outbox : délai d'annulation de quelques secondes, envoi programmé à une date, réessais, clé d'idempotence contre les doubles envois.
- Dossier Envoyés : ajout par IMAP APPEND, sauf chez les providers qui le font eux-mêmes (Gmail).

**Livrables** : tag `drafts` du contrat, constructeur MIME, ordonnanceur d'envoi.

**Critères de validation**

- [ ] Un mail reçu par un serveur SMTP de test est conforme (validation MIME, en-têtes de thread).
- [ ] La bonne identity est choisie dans au moins 6 cas de test (alias en Cc, Delivered-To seul, aucune correspondance…).
- [ ] Deux envois avec la même clé d'idempotence ne produisent qu'un mail.
- [ ] Un mail programmé part à la minute près, même après un redémarrage du serveur.
- [ ] Un envoi annulé dans le délai ne part jamais.

### S10 — Temps réel et notifications

**Objectif** : les clients ouverts reflètent chaque changement en moins d'une seconde, et chaque user choisit ce qui le notifie et avec quel détail.

**Dépend de** : S6, S8.

**Tâches**

- WebSocket `/api/v1/events` authentifié : abonnements par type, reprise après coupure depuis le dernier identifiant reçu, ping / pong.
- Événements : messages (reçu, modifié, déplacé, supprimé), compteurs de non-lus, état de synchro, brouillons, tâches longues (import, OCR…), notifications.
- Politique de notification par user : par account, mailbox, dossier global, tag ou règle ; niveau de détail (complet, expéditeur seul, compteur seul) ; heures calmes.
- Web Push (VAPID) optionnel : désactivé par défaut, interdit si l'admin le décide, contenu chiffré, avertissement sur les métadonnées.

**Livrables** : tag `events` du contrat, moteur de notification, catalogue des événements.

**Critères de validation**

- [ ] Un événement atteint un client connecté en moins d'une seconde.
- [ ] Après 30 secondes de coupure, le client récupère tous les événements manqués, sans doublon.
- [ ] Un dossier muet ne notifie rien ; le niveau « compteur seul » n'expose ni expéditeur ni sujet (tests).
- [ ] Web Push désactivé : aucun appel vers un service de push (test sur le module egress).

### S11 — Recherche

**Objectif** : un user retrouve n'importe quel mail en moins d'une demi-seconde, dans la boîte courante ou partout, avec des filtres combinables.

**Dépend de** : S7.

**Tâches**

- Index plein texte PostgreSQL sur sujet, expéditeur, destinataires, corps et noms de pièces jointes ; configurations `english` et `simple` avec `unaccent` ; trigrammes pour les fautes de frappe et les adresses.
- Langage de requête : `from:`, `to:`, `subject:`, `has:attachment`, `tag:`, `in:`, `account:`, `before:`, `after:`, `is:unread`, guillemets, exclusion par `-`.
- Portées : mailbox courante, account, tous les accounts, dossier global, vue.
- Recherches enregistrées, utilisables comme vues.
- Interface `SearchIndex` pour brancher Meilisearch ou OpenSearch sans toucher aux appelants.
- Mails PGP indexés seulement si l'user l'autorise.

**Livrables** : tag `search` du contrat, analyseur de requêtes, index.

**Critères de validation**

- [ ] L'analyseur de requêtes est couvert par des tests en table et un test de fuzzing.
- [ ] Sur 200 000 mails générés, 95 % des recherches répondent en moins de 500 ms (mesure consignée).
- [ ] Une recherche ne renvoie jamais le mail d'un autre user (test).

## Jalon 3 — Automatisation (S12 à S14)

À la fin du jalon, les mails se trient seuls par règles, déclenchent des scripts et des webhooks, et l'IA résume et classe dans le cadre fixé par l'admin. C'est le jalon le plus sensible en sécurité : on exécute du code utilisateur et on fait lire du contenu non fiable à un modèle.

### S12 — Moteur de règles

**Objectif** : un user automatise le tri de ses mails par des règles (regex comprises) qu'il peut essayer sur ses mails existants avant de les activer.

**Dépend de** : S8, S10.

**Tâches**

- Format de règle en JSON versionné : déclencheur (réception, envoi, manuel, planifié), arbre de conditions ET / OU / NON, actions ordonnées, priorité, option « ne pas appliquer les règles suivantes ».
- Conditions : expéditeur, destinataires, sujet, corps, en-têtes, account, mailbox, taille, date, pièces jointes (type, nom), tag, expéditeur connu, résultat SPF / DKIM. Opérateurs : égal, contient, commence par, regex (RE2, taille et durée bornées).
- Actions : déplacer, tagger, marquer, transférer, supprimer, notifier ou rendre muet, appeler un webhook ou un hook (S13), demander un classement ou un résumé à l'IA (S14). Une action inconnue est refusée à l'enregistrement.
- Exécution dans le pipeline de réception, après la synchro et avant la notification ; une règle ne s'applique qu'une fois par message et par version de règle.
- Mode essai : appliquer une règle à blanc sur les N derniers mails et renvoyer ce qu'elle aurait fait.
- Journal d'exécution par règle (mails touchés, actions, erreurs), consultable par l'user.
- Règles globales de l'admin, appliquées à tous les users (optionnel).

**Livrables** : tag `rules` du contrat, moteur, documentation du format.

**Critères de validation**

- [ ] Chaque condition et chaque action a ses tests ; les combinaisons ET / OU / NON sont testées en table.
- [ ] Une regex réputée catastrophique s'exécute en temps linéaire (test avec limite de durée).
- [ ] Le mode essai ne modifie rien (test qui compare l'état avant et après).
- [ ] Les règles ajoutent moins de 5 ms par message entrant sur 10 000 messages (mesure consignée).

### S13 — Hooks, webhooks et SDK

**Objectif** : l'admin et les users branchent leurs propres automatisations (scripts et appels HTTP) sur les événements mail, sans jamais pouvoir compromettre le serveur.

**Dépend de** : S12.

**Tâches**

- Webhooks : URL, événements choisis, signature HMAC-SHA256 avec horodatage contre le rejeu, réessais à délai croissant, désactivation après échecs répétés, adresses internes et privées refusées par défaut (protection SSRF).
- Hook-runner : conteneur séparé, sans secrets ni accès à la base, réseau coupé par défaut (ouvrable par l'admin), CPU, mémoire et durée limités, système de fichiers en lecture seule hors d'un dossier temporaire, utilisateur non root.
- Contrat d'un hook : l'événement arrive en JSON sur l'entrée standard, un jeton API restreint et de courte durée en variable d'environnement ; le hook renvoie un JSON d'actions ou appelle l'API via le SDK ; code de sortie et erreurs sont journalisés.
- Bibliothèque de hooks de l'admin et hooks personnels si la capability le permet : versions, activation, essai avec un événement d'exemple.
- Un hook d'analyse antivirus fourni comme exemple de référence.
- SDK générés depuis le contrat dans `sdks/python` et `sdks/rust`, plus une petite couche d'aide aux hooks (lire l'événement, renvoyer des actions). Publication sur PyPI et crates.io optionnelle.
- Exemples documentés : ranger les factures, poster dans une messagerie d'équipe, copier les pièces jointes ailleurs.

**Livrables** : tag `hooks` du contrat, image du hook-runner, deux SDK, exemples.

**Critères de validation**

- [ ] Un hook qui tente de lire un secret, d'ouvrir une connexion réseau ou de dépasser sa mémoire échoue proprement, et le journal le dit (tests d'évasion).
- [ ] Un hook ne peut rien faire au-delà des portées de son jeton (test).
- [ ] La vérification d'une signature de webhook tient en 5 lignes de Python, documentées et testées.
- [ ] Un webhook vers `127.0.0.1` ou une IP privée est refusé par défaut.
- [ ] Les SDK Python et Rust passent leurs tests contre un vrai serveur en conteneur.

### S14 — Couche IA

**Objectif** : l'IA résume, classe et trie avec le fournisseur choisi par l'admin, aucune donnée ne quitte le serveur sans accord explicite, et un mail piégé ne peut pas la détourner.

**Dépend de** : S12, S13.

**Tâches**

- Interface `AIProvider` (complétion, sortie JSON structurée, embeddings optionnels). Adaptateurs : Ollama, compatible OpenAI (vLLM, LM Studio, llama.cpp, services en ligne), Anthropic. Adaptateur « CLI » (Claude Code ou Codex installés sur le serveur) expérimental et désactivé par défaut, sous réserve des conditions d'usage des fournisseurs.
- Profil Docker `ai` avec Ollama et un petit modèle par défaut, choisi à l'implémentation selon le rapport qualité / mémoire.
- Politiques : providers IA permis par l'admin ; provider personnel si la capability le permet ; accord de l'user par account et par dossier ; mention « envoyé à un tiers » pour les providers externes ; masquage optionnel des adresses et numéros avant envoi.
- Fonctions : résumé d'un mail ou d'un thread, classement dans les tags existants, suggestion de dossier, résumé quotidien planifié, condition et action IA dans les règles.
- Hooks appelables par l'IA : seulement ceux d'une liste blanche, déclarés comme outils, avec confirmation de l'user pour les hooks marqués sensibles.
- Défense contre l'injection de prompt : le mail est encadré comme donnée non fiable, les sorties sont validées par schéma, aucune action hors liste autorisée.
- Cache des résultats (par message et version de prompt), suivi de consommation par user, quotas.
- Prompts versionnés dans des fichiers, en anglais.

**Livrables** : tag `ai` du contrat, adaptateurs, prompts, jeu d'évaluation.

**Critères de validation**

- [ ] IA désactivée : aucun appel vers un fournisseur IA (test sur le module egress).
- [ ] Un corpus d'au moins 20 mails piégés ne déclenche aucune action non autorisée.
- [ ] Le classement par tags atteint au moins 80 % d'accord avec un jeu annoté de 100 mails, sur le modèle local par défaut (mesure consignée).
- [ ] Un résultat déjà calculé n'est pas redemandé au fournisseur (test).
- [ ] Les tests tournent sans réseau grâce à un faux provider déterministe.

## Jalon 4 — Fonctions avancées (S15 à S18)

À la fin du jalon, MagpieMail couvre toutes les notes : PGP, antivirus, Mode Document, calendrier, contacts, import et export. Ces quatre phases sont largement indépendantes entre elles et peuvent être réordonnées selon tes priorités.

### S15 — Sécurité du contenu : PGP et antivirus

**Objectif** : les mails PGP se lisent et s'envoient signés et chiffrés, et chaque pièce jointe peut être analysée avant d'être servie.

**Dépend de** : S9.

**Tâches**

- Trousseau par user : import de clés publiques et privées, génération, export, révocation ; clé privée protégée par sa phrase de passe et par la clé du user.
- Déverrouillage : la phrase de passe ouvre la clé pour la durée de la session (réglable), sans jamais être stockée. Le déchiffrement côté client est écarté pour l'instant ; l'ADR explique pourquoi.
- Lecture : PGP/MIME et PGP inline ; statut de signature clair (valide, clé inconnue, invalide, expirée).
- Envoi : signer, chiffrer ou les deux ; chiffrement aussi pour soi afin de garder une copie lisible ; alerte si un destinataire n'a pas de clé.
- Découverte de clés (WKD, keys.openpgp.org) seulement si l'user l'active, via le module egress.
- Antivirus : client ClamAV (protocole clamd) dans un profil Docker `antivirus`, analyse à la réception et avant téléchargement, statut par pièce jointe (saine, infectée, non analysée), quarantaine. Sans ClamAV, le hook d'analyse de S13 prend le relais.

**Livrables** : tag `pgp` du contrat, trousseau, intégration ClamAV.

**Critères de validation**

- [ ] Interopérabilité dans les deux sens avec GnuPG : chiffrement, déchiffrement, signature, vérification.
- [ ] Une signature falsifiée est signalée invalide.
- [ ] La clé privée n'est jamais écrite en clair (test).
- [ ] Le fichier de test EICAR est détecté et mis en quarantaine ; on ne peut plus le télécharger sans action explicite.
- [ ] Sans ClamAV, les pièces jointes sont marquées « non analysées » et tout le reste fonctionne.

### S16 — Mode Document

**Objectif** : pièces jointes et documents déposés forment une bibliothèque où l'on retrouve un fichier par son contenu, même scanné.

**Dépend de** : S11 ; S14 pour les fonctions IA.

**Tâches**

- Modèle Document : blob, origine (pièce jointe de tel mail ou dépôt manuel), titre, description, mots-clés, tags, date du document, langue.
- Alimentation automatique depuis les pièces jointes, filtrable (types, taille minimale, accounts, images de signature exclues) ; dépôt manuel par l'API.
- Pipeline en tâches de fond : détection du type, extraction de texte (Apache Tika), OCR des images et PDF scannés (OCRmyPDF / Tesseract, langues configurables), vignettes.
- Conteneurs `tika` et `ocr` dans un profil Docker `documents`.
- IA optionnelle : résumé, description, mots-clés, tags proposés (existants, ou nouveaux à valider par l'user).
- Recherche sur texte extrait, description, mots-clés et tags, avec les opérateurs de S11.
- Déduplication : une même facture reçue deux fois donne un document avec deux origines.

**Livrables** : tag `documents` du contrat, pipeline, profil Docker.

**Critères de validation**

- [ ] Un PDF scanné de test est retrouvé par un mot présent uniquement dans l'image.
- [ ] Après un redémarrage, le pipeline reprend sans retraiter ce qui est fini.
- [ ] Sans les conteneurs optionnels, le Mode Document fonctionne sans OCR et le signale.
- [ ] Un document de 50 pages est traité en moins de 2 minutes sur la machine de dev (mesure consignée).

### S17 — Calendrier et contacts

**Objectif** : rendez-vous et contacts des accounts sont synchronisés et servent dans les mails (invitations, autocomplétion, règles).

**Dépend de** : S9.

**Tâches**

- Calendriers : CalDAV (générique et Google), Microsoft Graph pour Outlook, calendriers locaux MagpieMail.
- Événements : création, modification, suppression, récurrences, fuseaux horaires, rappels via le moteur de notification.
- Invitations reçues par mail (iCalendar / iMIP) : détection, aperçu, réponse accepter / refuser / peut-être, mise à jour de l'événement.
- Contacts : CardDAV (et API Google ou Microsoft si nécessaire), contacts locaux, collecte des correspondants (désactivable), fusion des doublons.
- Autocomplétion des destinataires ; condition « expéditeur connu » pour les règles.
- Import et export ICS et vCard.

**Livrables** : tags `calendar` et `contacts` du contrat, adaptateurs.

**Critères de validation**

- [ ] Synchro dans les deux sens testée contre un serveur CalDAV / CardDAV en conteneur (Radicale ou Baïkal).
- [ ] Une invitation Google et une invitation Outlook s'affichent, et la réponse envoyée est conforme.
- [ ] Récurrences et changements d'heure couverts par des tests en table.

### S18 — Import et export

**Objectif** : un user ou l'admin fait entrer et sortir toutes les données dans des formats standards, pour sauvegarder, migrer ou alimenter un autre provider.

**Dépend de** : S9, S17.

**Tâches**

- Import : EML, MBOX, Maildir, PST (readpst en conteneur), ICS, vCard ; vers un account, une mailbox ou un dossier global.
- Export : un mail (EML, PDF imprimable), un dossier ou une recherche (MBOX, Maildir, ZIP d'EML), tout un user (archive ZIP : mails, métadonnées JSON, tags, règles, hooks, calendriers, contacts, documents).
- Copie directe vers un autre account IMAP (changement de provider), avec reprise.
- Export complet d'un user par l'admin, journalisé.
- Tâches longues reprenables, progression en temps réel, fichiers d'export à durée de vie limitée.
- Format d'archive documenté, versionné et réimportable.

**Livrables** : tag `transfer` du contrat, importeurs, exporteurs, documentation du format.

**Critères de validation**

- [ ] Exporter un user puis l'importer sur une instance neuve redonne les mêmes messages, tags, dossiers et règles (test).
- [ ] Un MBOX de 1 Go s'importe sans dépasser 512 Mo de mémoire.
- [ ] Une migration IMAP interrompue reprend sans doublon.

## Jalon 5 — Release (S19)

### S19 — Durcissement, documentation et release

**Objectif** : une instance MagpieMail s'installe en 15 minutes avec la doc, résiste aux attaques courantes, se sauvegarde et se met à jour sans perte.

**Dépend de** : toutes les phases retenues.

**Tâches**

- Revue de sécurité complète selon l'OWASP ASVS niveau 2, corrections, `SECURITY.md` (signalement de failles).
- Fuzzing des analyseurs (MIME, HTML, requêtes de recherche, règles) chaque nuit en CI.
- Tests de charge (k6) sur 20 users, 50 accounts et 1 million de mails ; profilage et corrections.
- Observabilité en option : métriques Prometheus, traces OpenTelemetry, tableau de bord Grafana d'exemple.
- Sauvegarde et restauration : script (base + blobs), rappel insistant de sauvegarder la clé maître hors du serveur, test de restauration automatisé.
- Mises à jour : migrations au démarrage (désactivables) ; comparaison automatique du contrat avec la version précédente pour bloquer tout changement cassant dans `/api/v1`.
- Livraison : images amd64 et arm64, SBOM, images signées (cosign), notes de version tirées du changelog.
- Documentation : installation (compose de référence avec tous les profils), configuration, OAuth Google et Microsoft, Proton Bridge, Gluetun, IA locale, hooks et SDK, sauvegarde, mise à jour, FAQ.

**Livrables** : version 1.0.0, documentation complète, images signées.

**Critères de validation**

- [ ] Une personne qui suit la doc installe une instance fonctionnelle en moins de 15 minutes (essai réel chronométré).
- [ ] Une restauration complète sur une machine vierge retrouve toutes les données (test automatisé).
- [ ] Aucune vulnérabilité haute ou critique connue dans les dépendances et l'image.
- [ ] La mise à jour depuis la version précédente passe sans intervention manuelle (test).
- [ ] Le fuzzing de nuit tourne depuis une semaine sans plantage non corrigé.
