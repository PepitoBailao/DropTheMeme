# DropTheMeme 🐸

> Overlay Steam pour partager des mèmes, sons et vidéos avec tes amis pendant que tu joues.

## Prérequis

- [Go 1.22+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- [Rust + Tauri CLI v2](https://tauri.app/start/prerequisites/)
- [PostgreSQL 16+](https://www.postgresql.org/)
- Une clé API Steam ([steam.dev](https://steamcommunity.com/dev/apikey))

---

## Installation rapide

### 1. Backend Go

```bash
cd backend
cp .env.example .env
# Remplis .env avec ta DATABASE_URL, STEAM_API_KEY, JWT_SECRET

# Créer la base de données
psql -U postgres -c "CREATE DATABASE dropthememe;"
psql -U postgres -d dropthememe -f db/schema.sql

# Installer les dépendances et lancer
go mod tidy
go run ./cmd/server
# → Serveur sur http://localhost:8080
```

### 2. Frontend Tauri

```bash
cd frontend
cp .env.example .env
# VITE_API_URL=http://localhost:8080
# VITE_WS_URL=ws://localhost:8080

npm install
npm run tauri:dev
```

---

## Structure du projet

```
dropthememe/
├── backend/
│   ├── cmd/server/main.go       # Point d'entrée
│   ├── config/config.go         # Variables d'environnement
│   ├── db/schema.sql            # Schéma PostgreSQL
│   └── internal/
│       ├── auth/auth.go         # Steam + JWT
│       ├── ws/hub.go            # WebSocket Hub
│       ├── media/               # Upload médias (V2)
│       └── db/db.go             # Connexion PostgreSQL
└── frontend/
    ├── src/
    │   ├── api.ts               # Client HTTP + WebSocket
    │   ├── store.ts             # Stockage token Tauri
    │   ├── App.tsx              # Fenêtre principale
    │   └── Overlay.tsx          # Overlay in-game
    └── src-tauri/
        └── tauri.conf.json      # Config Tauri
```

---

## Routes API

| Méthode | Route | Auth | Description |
|---------|-------|------|-------------|
| GET | `/health` | ✗ | Health check |
| POST | `/auth/steam` | ✗ | Connexion via ticket Steam → JWT |
| GET | `/groups` | ✓ | Mes groupes |
| POST | `/groups` | ✓ | Créer un groupe |
| POST | `/groups/join` | ✓ | Rejoindre via code |
| GET | `/groups/{id}/members` | ✓ | Membres connectés |
| GET | `/ws?group_id=...` | ✓ | WebSocket temps réel |

---

## Contribuer

Issues et PRs bienvenues sur GitHub.

## Licence

MIT
