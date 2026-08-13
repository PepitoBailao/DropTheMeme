package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// SteamClaims est ce qu'on stocke dans le JWT
type SteamClaims struct {
	UserID   string `json:"user_id"`
	SteamID  string `json:"steam_id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	jwt.RegisteredClaims
}

// VerifySteamTicket vérifie un ticket d'auth Steam côté serveur
// Le client Tauri génère ce ticket via steamworks-rs et l'envoie au backend
func VerifySteamTicket(apiKey, appID, ticket string) (string, error) {
	params := url.Values{}
	params.Set("key", apiKey)
	params.Set("appid", appID)
	params.Set("ticket", ticket)

	resp, err := http.Get("https://api.steampowered.com/ISteamUserAuth/AuthenticateUserTicket/v1/?" + params.Encode())
	if err != nil {
		return "", fmt.Errorf("appel Steam API échoué : %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Response struct {
			Params struct {
				Result          string `json:"result"`
				SteamID         string `json:"steamid"`
				OwnerSteamID    string `json:"ownersteamid"`
				VACBanned       bool   `json:"vacbanned"`
				PublisherBanned bool   `json:"publisherbanned"`
			} `json:"params"`
			Error *struct {
				ErrorCode int    `json:"errorcode"`
				ErrorDesc string `json:"errordesc"`
			} `json:"error"`
		} `json:"response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing réponse Steam échoué : %w", err)
	}

	if result.Response.Error != nil {
		return "", fmt.Errorf("Steam refuse le ticket : %s", result.Response.Error.ErrorDesc)
	}

	if result.Response.Params.Result != "OK" {
		return "", fmt.Errorf("ticket Steam invalide")
	}

	return result.Response.Params.SteamID, nil
}

// GetSteamProfile récupère le pseudo et l'avatar d'un SteamID
func GetSteamProfile(apiKey, steamID string) (username, avatarURL string, err error) {
	params := url.Values{}
	params.Set("key", apiKey)
	params.Set("steamids", steamID)

	resp, err := http.Get("https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?" + params.Encode())
	if err != nil {
		return "", "", fmt.Errorf("appel Steam API profil échoué : %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Response struct {
			Players []struct {
				SteamID   string `json:"steamid"`
				Personame string `json:"personaname"`
				Avatar    string `json:"avatarfull"`
			} `json:"players"`
		} `json:"response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("parsing profil Steam échoué : %w", err)
	}

	if len(result.Response.Players) == 0 {
		return "", "", fmt.Errorf("profil Steam introuvable")
	}

	p := result.Response.Players[0]
	return p.Personame, p.Avatar, nil
}

// IssueJWT génère un JWT signé pour un utilisateur
func IssueJWT(secret, userID, steamID, username, avatar string) (string, error) {
	claims := SteamClaims{
		UserID:   userID,
		SteamID:  steamID,
		Username: username,
		Avatar:   avatar,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseJWT vérifie et décode un JWT
func ParseJWT(secret, tokenString string) (*SteamClaims, error) {
	// Nettoyer le préfixe "Bearer " si présent
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenString, &SteamClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("méthode de signature inattendue : %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("JWT invalide : %w", err)
	}

	claims, ok := token.Claims.(*SteamClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("claims JWT invalides")
	}

	return claims, nil
}

// Middleware JWT pour les routes protégées
func Middleware(secret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			http.Error(w, "token manquant", http.StatusUnauthorized)
			return
		}

		claims, err := ParseJWT(secret, tokenStr)
		if err != nil {
			http.Error(w, "token invalide", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		next(w, r.WithContext(ctx))
	}
}

type claimsKey struct{}

// GetClaims extrait les claims du contexte
func GetClaims(r *http.Request) *SteamClaims {
	claims, _ := r.Context().Value(claimsKey{}).(*SteamClaims)
	return claims
}
