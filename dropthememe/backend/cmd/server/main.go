package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/dropthememe/backend/config"
	"github.com/dropthememe/backend/internal/auth"
	"github.com/dropthememe/backend/internal/db"
	"github.com/dropthememe/backend/internal/ws"
)

func main() {
	cfg := config.Load()

	// Connexion PostgreSQL
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("❌ PostgreSQL : %v", err)
	}
	defer db.Close()

	// Hub WebSocket
	hub := ws.NewHub()
	go hub.Run()

	// Routes
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Auth Steam : le client envoie son ticket, on retourne un JWT
	mux.HandleFunc("POST /auth/steam", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Ticket string `json:"ticket"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Ticket == "" {
			http.Error(w, "ticket manquant", http.StatusBadRequest)
			return
		}

		// Vérifier le ticket Steam
		steamID, err := auth.VerifySteamTicket(cfg.SteamAPIKey, cfg.SteamAppID, body.Ticket)
		if err != nil {
			log.Printf("Ticket Steam invalide : %v", err)
			http.Error(w, "ticket Steam invalide", http.StatusUnauthorized)
			return
		}

		// Récupérer le profil Steam
		username, avatarURL, err := auth.GetSteamProfile(cfg.SteamAPIKey, steamID)
		if err != nil {
			log.Printf("Profil Steam introuvable : %v", err)
			http.Error(w, "profil Steam introuvable", http.StatusInternalServerError)
			return
		}

		// Upsert utilisateur en base
		var userID string
		err = db.Pool.QueryRow(context.Background(), `
			INSERT INTO users (steam_id, username, avatar_url, last_seen)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (steam_id) DO UPDATE
				SET username = EXCLUDED.username,
				    avatar_url = EXCLUDED.avatar_url,
				    last_seen = NOW()
			RETURNING id
		`, steamID, username, avatarURL).Scan(&userID)
		if err != nil {
			log.Printf("Upsert user échoué : %v", err)
			http.Error(w, "erreur serveur", http.StatusInternalServerError)
			return
		}

		// Émettre le JWT
		token, err := auth.IssueJWT(cfg.JWTSecret, userID, steamID, username, avatarURL)
		if err != nil {
			http.Error(w, "erreur génération token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token":    token,
			"user_id":  userID,
			"username": username,
			"avatar":   avatarURL,
		})
	})

	// Créer un groupe
	mux.HandleFunc("POST /groups", auth.Middleware(cfg.JWTSecret, func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r)

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "nom du groupe manquant", http.StatusBadRequest)
			return
		}

		inviteCode := uuid.NewString()[:8]
		var groupID string
		err := db.Pool.QueryRow(context.Background(), `
			INSERT INTO groups (name, invite_code, owner_id)
			VALUES ($1, $2, $3)
			RETURNING id
		`, body.Name, inviteCode, claims.UserID).Scan(&groupID)
		if err != nil {
			log.Printf("Création groupe échouée : %v", err)
			http.Error(w, "erreur serveur", http.StatusInternalServerError)
			return
		}

		// Ajouter le créateur comme membre
		db.Pool.Exec(context.Background(), `
			INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)
		`, groupID, claims.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"group_id":    groupID,
			"invite_code": inviteCode,
		})
	}))

	// Rejoindre un groupe via code d'invitation
	mux.HandleFunc("POST /groups/join", auth.Middleware(cfg.JWTSecret, func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r)

		var body struct {
			InviteCode string `json:"invite_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.InviteCode == "" {
			http.Error(w, "code d'invitation manquant", http.StatusBadRequest)
			return
		}

		var groupID, groupName string
		err := db.Pool.QueryRow(context.Background(), `
			SELECT id, name FROM groups WHERE invite_code = $1
		`, body.InviteCode).Scan(&groupID, &groupName)
		if err != nil {
			http.Error(w, "code d'invitation invalide", http.StatusNotFound)
			return
		}

		db.Pool.Exec(context.Background(), `
			INSERT INTO group_members (group_id, user_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, groupID, claims.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"group_id": groupID,
			"name":     groupName,
		})
	}))

	// Mes groupes
	mux.HandleFunc("GET /groups", auth.Middleware(cfg.JWTSecret, func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r)

		rows, err := db.Pool.Query(context.Background(), `
			SELECT g.id, g.name, g.invite_code
			FROM groups g
			JOIN group_members gm ON gm.group_id = g.id
			WHERE gm.user_id = $1
			ORDER BY g.created_at DESC
		`, claims.UserID)
		if err != nil {
			http.Error(w, "erreur serveur", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Group struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			InviteCode  string `json:"invite_code"`
		}
		var groups []Group
		for rows.Next() {
			var g Group
			rows.Scan(&g.ID, &g.Name, &g.InviteCode)
			groups = append(groups, g)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(groups)
	}))

	// WebSocket : rejoindre un groupe en temps réel
	mux.HandleFunc("GET /ws", auth.Middleware(cfg.JWTSecret, func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r)
		groupID := r.URL.Query().Get("group_id")
		if groupID == "" {
			http.Error(w, "group_id manquant", http.StatusBadRequest)
			return
		}

		// Vérifier que l'utilisateur est bien membre du groupe
		var exists bool
		db.Pool.QueryRow(context.Background(), `
			SELECT EXISTS(
				SELECT 1 FROM group_members
				WHERE group_id = $1 AND user_id = $2
			)
		`, groupID, claims.UserID).Scan(&exists)

		if !exists {
			http.Error(w, "tu n'es pas membre de ce groupe", http.StatusForbidden)
			return
		}

		hub.ServeWS(w, r, claims.UserID, claims.Username, claims.Avatar, groupID, uuid.NewString())
	}))

	// Membres connectés d'un groupe
	mux.HandleFunc("GET /groups/{id}/members", auth.Middleware(cfg.JWTSecret, func(w http.ResponseWriter, r *http.Request) {
		groupID := r.PathValue("id")
		members := hub.GetGroupMembers(groupID)
		if members == nil {
			members = []map[string]string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(members)
	}))

	// Démarrage serveur
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 DropTheMeme backend démarré sur :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Serveur : %v", err)
		}
	}()

	// Arrêt propre
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Arrêt du serveur...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
