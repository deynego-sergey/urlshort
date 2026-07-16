package pg

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
)

func DatabaseConnect(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
		return nil, err
	}
	//defer conn.Close(ctx)

	// Example query to test connection
	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		log.Fatalf("Query failed: %v", err)
		return nil, err
	}

	log.Println("Connected to:", version)
	return conn, nil
}
