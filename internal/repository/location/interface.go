package location

import (
	"context"
	"errors"
)

// ErrNotFound is returned when no location matches.
var ErrNotFound = errors.New("location not found")

// MapPose is overworld placement.
type MapPose struct {
	X     float64
	Z     float64
	Scale float64
}

// Assets references billboard / icon art.
type Assets struct {
	Icon string
}

// Location is a board / overworld place document.
type Location struct {
	ID                string
	WorldID           string
	Slug              string
	Name              string
	CountryCode       string
	Region            string
	Kind              string
	BoardIndex        int
	BoardCode         string
	Price             int
	Rents             []int
	HouseCost         *int
	ColorGroup        *string
	SpecialType       *string
	UtilityMultiplier []int
	TaxAmount         *int
	PassBonus         *int
	Map               MapPose
	Assets            Assets
	HubID             string
	EnterRadius       float64
	Description       string
	Svgcities         string
	SvgcitiesPath     string
	SvgcitiesURL      string
	Attribution       string
	Symbol            string
	SymbolStory       string
	About             string
	AboutShort        string
	Tags              []string
	LandmarkCategory  string
}

// WorldSummary is a distinct world pack.
type WorldSummary struct {
	WorldID string
	Count   int
}

// Repository persists locations in MongoDB.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	ListByWorldID(ctx context.Context, worldID string) ([]Location, error)
	FindByID(ctx context.Context, id string) (*Location, error)
	FindByWorldAndSlug(ctx context.Context, worldID, slug string) (*Location, error)
	ListWorlds(ctx context.Context) ([]WorldSummary, error)
	// ReplaceAll deletes existing docs and inserts seed docs (CLI seed).
	ReplaceAll(ctx context.Context, docs []Location) (inserted int, err error)
}
