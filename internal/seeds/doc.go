// Package seeds holds offline tooling for seeds/locations.json (remap, polish, CLI seed).
package seeds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	locationrepo "meetopoly-be/internal/repository/location"
)

// MapPose is overworld placement in the seed file.
type MapPose struct {
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Scale float64 `json:"scale"`
}

// Assets references billboard / icon art.
type Assets struct {
	Icon string `json:"icon"`
}

// LocationDoc is one entry in seeds/locations.json.
type LocationDoc struct {
	WorldID           string   `json:"worldId"`
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	CountryCode       string   `json:"countryCode,omitempty"`
	Region            string   `json:"region,omitempty"`
	Kind              string   `json:"kind"`
	BoardIndex        int      `json:"boardIndex"`
	BoardCode         string   `json:"boardCode"`
	Price             int      `json:"price"`
	Rents             []int    `json:"rents"`
	HouseCost         *int     `json:"houseCost,omitempty"`
	ColorGroup        *string  `json:"colorGroup"`
	SpecialType       *string  `json:"specialType,omitempty"`
	UtilityMultiplier []int    `json:"utilityMultiplier,omitempty"`
	TaxAmount         *int     `json:"taxAmount,omitempty"`
	PassBonus         *int     `json:"passBonus,omitempty"`
	Map               MapPose  `json:"map"`
	Assets            Assets   `json:"assets"`
	HubID             string   `json:"hubId"`
	EnterRadius       float64  `json:"enterRadius"`
	Description       string   `json:"description,omitempty"`
	Svgcities         string   `json:"svgcities,omitempty"`
	SvgcitiesPath     string   `json:"svgcitiesPath,omitempty"`
	SvgcitiesURL      string   `json:"svgcitiesUrl,omitempty"`
	Attribution       string   `json:"attribution,omitempty"`
	Symbol            string   `json:"symbol,omitempty"`
	SymbolStory       string   `json:"symbolStory,omitempty"`
	About             string   `json:"about,omitempty"`
	AboutShort        string   `json:"aboutShort,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	LandmarkCategory  string   `json:"landmarkCategory,omitempty"`
}

// LoadFile reads a JSON array of location docs.
func LoadFile(path string) ([]LocationDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var docs []LocationDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, fmt.Errorf("parse seed JSON: %w", err)
	}
	return docs, nil
}

// WriteFile writes location docs as indented JSON with a trailing newline.
func WriteFile(path string, docs []LocationDoc) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(docs); err != nil {
		return err
	}
	// Encode already adds a trailing newline.
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Clone deep-copies a location doc (via JSON round-trip).
func Clone(d LocationDoc) LocationDoc {
	b, err := json.Marshal(d)
	if err != nil {
		return d
	}
	var out LocationDoc
	if err := json.Unmarshal(b, &out); err != nil {
		return d
	}
	return out
}

// GroupByWorld groups docs by worldId (stable world order not guaranteed).
func GroupByWorld(docs []LocationDoc) map[string][]LocationDoc {
	out := make(map[string][]LocationDoc)
	for _, d := range docs {
		out[d.WorldID] = append(out[d.WorldID], d)
	}
	return out
}

// SortedWorldIDs returns sorted world keys.
func SortedWorldIDs(byWorld map[string][]LocationDoc) []string {
	ids := make([]string, 0, len(byWorld))
	for id := range byWorld {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ToRepo converts a seed doc into a repository Location.
func ToRepo(d LocationDoc) locationrepo.Location {
	return locationrepo.Location{
		WorldID:           d.WorldID,
		Slug:              d.Slug,
		Name:              d.Name,
		CountryCode:       d.CountryCode,
		Region:            d.Region,
		Kind:              d.Kind,
		BoardIndex:        d.BoardIndex,
		BoardCode:         d.BoardCode,
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

func strPtr(s string) *string { return &s }

func specialType(d LocationDoc) string {
	if d.SpecialType == nil {
		return ""
	}
	return *d.SpecialType
}
