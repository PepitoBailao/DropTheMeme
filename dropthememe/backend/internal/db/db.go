package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Connect(databaseURL string) error {
	var err error
	Pool, err = pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("impossible de se connecter à PostgreSQL : %w", err)
	}

	if err = Pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping PostgreSQL échoué : %w", err)
	}

	log.Println("✅ PostgreSQL connecté")
	return nil
}

func Close() {
	if Pool != nil {
		Pool.Close()
	}
}
