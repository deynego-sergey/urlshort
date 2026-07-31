package mongoatlas

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Client struct {
	client *mongo.Client
	dbName string
}

func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	opts := options.Client().ApplyURI(cfg.URI)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongo atlas v2: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping mongo atlas v2: %w", err)
	}

	return &Client{
		client: client,
		dbName: cfg.DBName,
	}, nil
}

func (c *Client) Database() *mongo.Database {
	return c.client.Database(c.dbName)
}

func (c *Client) Close(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}
