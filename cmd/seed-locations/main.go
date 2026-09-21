package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"meetopoly-be/internal/platform/config"
	mongoplatform "meetopoly-be/internal/platform/mongo"
	locationrepo "meetopoly-be/internal/repository/location"
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

	raw, err := os.ReadFile(*seedPath)
	if err != nil {
		log.Fatalf("read seed file: %v", err)
	}

	var docs []seedDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		log.Fatalf("parse seed JSON: %v", err)
	}

	locs := make([]locationrepo.Location, 0, len(docs))
	for i := range docs {
		locs = append(locs, docs[i].toLocation())
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

type seedMap struct {
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Scale float64 `json:"scale"`
}

type seedAssets struct {
	Icon string `json:"icon"`
}

type seedDoc struct {
	WorldID           string      `json:"worldId"`
	Slug              string      `json:"slug"`
	Name              string      `json:"name"`
	CountryCode       string      `json:"countryCode"`
	Region            string      `json:"region"`
	Kind              string      `json:"kind"`
	BoardIndex        int         `json:"boardIndex"`
	Price             int         `json:"price"`
	Rents             []int       `json:"rents"`
	HouseCost         *int        `json:"houseCost"`
	ColorGroup        *string     `json:"colorGroup"`
	SpecialType       *string     `json:"specialType"`
	UtilityMultiplier []int       `json:"utilityMultiplier"`
	TaxAmount         *int        `json:"taxAmount"`
	PassBonus         *int        `json:"passBonus"`
	Map               seedMap     `json:"map"`
	Assets            seedAssets  `json:"assets"`
	HubID             string      `json:"hubId"`
	EnterRadius       float64     `json:"enterRadius"`
	Description       string      `json:"description"`
	Svgcities         string      `json:"svgcities"`
	SvgcitiesPath     string      `json:"svgcitiesPath"`
	SvgcitiesURL      string      `json:"svgcitiesUrl"`
	Attribution       string      `json:"attribution"`
	Symbol            string      `json:"symbol"`
	SymbolStory       string      `json:"symbolStory"`
	About             string      `json:"about"`
	AboutShort        string      `json:"aboutShort"`
	Tags              []string    `json:"tags"`
	LandmarkCategory  string      `json:"landmarkCategory"`
}

func (d seedDoc) toLocation() locationrepo.Location {
	return locationrepo.Location{
		WorldID:           d.WorldID,
		Slug:              d.Slug,
		Name:              d.Name,
		CountryCode:       d.CountryCode,
		Region:            d.Region,
		Kind:              d.Kind,
		BoardIndex:        d.BoardIndex,
		Price:             d.Price,
		Rents:             d.Rents,
		HouseCost:         d.HouseCost,
		ColorGroup:        d.ColorGroup,
		SpecialType:       d.SpecialType,
		UtilityMultiplier: d.UtilityMultiplier,
		TaxAmount:         d.TaxAmount,
		PassBonus:         d.PassBonus,
		Map: locationrepo.MapPose{
			X:     d.Map.X,
			Z:     d.Map.Z,
			Scale: d.Map.Scale,
		},
		Assets:           locationrepo.Assets{Icon: d.Assets.Icon},
		HubID:            d.HubID,
		EnterRadius:      d.EnterRadius,
		Description:      d.Description,
		Svgcities:        d.Svgcities,
		SvgcitiesPath:    d.SvgcitiesPath,
		SvgcitiesURL:     d.SvgcitiesURL,
		Attribution:      d.Attribution,
		Symbol:           d.Symbol,
		SymbolStory:      d.SymbolStory,
		About:            d.About,
		AboutShort:       d.AboutShort,
		Tags:             d.Tags,
		LandmarkCategory: d.LandmarkCategory,
	}
}
