package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"meetopoly-be/internal/platform/config"
	mongoplatform "meetopoly-be/internal/platform/mongo"
	locationrepo "meetopoly-be/internal/repository/location"
	"meetopoly-be/internal/seeds"
)

func main() {
	seedPath := flag.String("file", "seeds/locations.json", "path to locations JSON array")
	flag.Parse()

	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := mongoplatform.Connect(ctx, cfg.MongoURI, cfg.PingTimeout)
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
	}()

	docs, err := seeds.LoadFile(*seedPath)
	if err != nil {
		log.Fatalf("read seed file: %v", err)
	}

	locs := make([]locationrepo.Location, 0, len(docs))
	for _, d := range docs {
		locs = append(locs, seeds.ToRepo(d))
	}

	repo := locationrepo.NewMongoRepository(client.Database(cfg.MongoDatabase))
	if err := repo.EnsureIndexes(ctx); err != nil {
		log.Fatalf("indexes: %v", err)
	}

	n, err := repo.ReplaceAll(ctx, locs)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	fmt.Printf("seeded %d locations into %s.locations from %s\n", n, cfg.MongoDatabase, *seedPath)
}
