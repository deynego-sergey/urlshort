package mongoatlas

import (
	"fmt"
	"os"
)

type Config struct {
	URI    string
	DBName string
}

func LoadConfigFromEnv() (*Config, error) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		return nil, fmt.Errorf("MONGO_URI environment variable is required")
	}

	dbName := os.Getenv("MONGO_DB_NAME")
	if dbName == "" {
		return nil, fmt.Errorf("MONGO_DB_NAME environment variable is required")
	}

	return &Config{
		URI:    uri,
		DBName: dbName,
	}, nil
}
