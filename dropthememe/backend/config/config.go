package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Env         string
	DatabaseURL string
	JWTSecret   string
	SteamAPIKey string
	SteamAppID  string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Pas de fichier .env trouvé, on utilise les variables d'environnement système")
	}

	return &Config{
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		SteamAPIKey: getEnv("STEAM_API_KEY", ""),
		SteamAppID:  getEnv("STEAM_APP_ID", ""),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
