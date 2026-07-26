package mongoatlas

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Client struct {
	*mongo.Client
	db *mongo.Database
}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.URI)

	client, err := mongo.Connect(ctxTimeout, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb atlas: %w", err)
	}

	if err := client.Ping(ctxTimeout, nil); err != nil {
		_ = client.Disconnect(ctxTimeout)
		return nil, fmt.Errorf("failed to ping mongodb atlas: %w", err)
	}

	return &Client{
		Client: client,
		db:     client.Database(cfg.DBName),
	}, nil
}

func (c *Client) Database() *mongo.Database {
	return c.db
}

func (c *Client) Close(ctx context.Context) error {
	if c.Client == nil {
		return nil
	}
	return c.Disconnect(ctx)
}
