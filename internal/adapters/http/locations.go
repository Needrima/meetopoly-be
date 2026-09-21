package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	locationrepo "meetopoly-be/internal/repository/location"
	locationsvc "meetopoly-be/internal/services/location"
)

type worldSummaryResponse struct {
	WorldID string `json:"worldId"`
	Count   int    `json:"count"`
}

type worldsResponse struct {
	Worlds []worldSummaryResponse `json:"worlds"`
}

type locationsResponse struct {
	Locations []locationResponse `json:"locations"`
}

type mapPoseResponse struct {
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Scale float64 `json:"scale"`
}

type assetsResponse struct {
	Icon string `json:"icon"`
}

type locationResponse struct {
	ID                string          `json:"id"`
	WorldID           string          `json:"worldId"`
	Slug              string          `json:"slug"`
	Name              string          `json:"name"`
	CountryCode       string          `json:"countryCode,omitempty"`
	Region            string          `json:"region,omitempty"`
	Kind              string          `json:"kind"`
	BoardIndex        int             `json:"boardIndex"`
	Price             int             `json:"price"`
	Rents             []int           `json:"rents,omitempty"`
	HouseCost         *int            `json:"houseCost,omitempty"`
	ColorGroup        *string         `json:"colorGroup,omitempty"`
	SpecialType       *string         `json:"specialType,omitempty"`
	UtilityMultiplier []int           `json:"utilityMultiplier,omitempty"`
	TaxAmount         *int            `json:"taxAmount,omitempty"`
	PassBonus         *int            `json:"passBonus,omitempty"`
	Map               mapPoseResponse `json:"map"`
	Assets            assetsResponse  `json:"assets"`
	HubID             string          `json:"hubId"`
	EnterRadius       float64         `json:"enterRadius"`
	Description       string          `json:"description,omitempty"`
	Svgcities         string          `json:"svgcities,omitempty"`
	SvgcitiesPath     string          `json:"svgcitiesPath,omitempty"`
	SvgcitiesURL      string          `json:"svgcitiesUrl,omitempty"`
	Attribution       string          `json:"attribution,omitempty"`
	Symbol            string          `json:"symbol,omitempty"`
	SymbolStory       string          `json:"symbolStory,omitempty"`
	About             string          `json:"about,omitempty"`
	AboutShort        string          `json:"aboutShort,omitempty"`
	Tags              []string        `json:"tags,omitempty"`
	LandmarkCategory  string          `json:"landmarkCategory,omitempty"`
}

func handleListWorlds(svc locationsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		worlds, err := svc.ListWorlds(r.Context())
		if err != nil {
			mapLocationError(w, err)
			return
		}
		out := make([]worldSummaryResponse, 0, len(worlds))
		for _, world := range worlds {
			out = append(out, worldSummaryResponse{
				WorldID: world.WorldID,
				Count:   world.Count,
			})
		}
		writeJSON(w, http.StatusOK, worldsResponse{Worlds: out})
	}
}

func handleListLocations(svc locationsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		worldID := r.URL.Query().Get("worldId")
		locs, err := svc.ListByWorldID(r.Context(), worldID)
		if err != nil {
			mapLocationError(w, err)
			return
		}
		out := make([]locationResponse, 0, len(locs))
		for i := range locs {
			out = append(out, toLocationResponse(&locs[i]))
		}
		writeJSON(w, http.StatusOK, locationsResponse{Locations: out})
	}
}

func handleGetLocationByID(svc locationsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "locationId")
		loc, err := svc.GetByID(r.Context(), id)
		if err != nil {
			mapLocationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toLocationResponse(loc))
	}
}

func handleGetLocationBySlug(svc locationsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		worldID := r.URL.Query().Get("worldId")
		slug := r.URL.Query().Get("slug")
		loc, err := svc.GetByWorldAndSlug(r.Context(), worldID, slug)
		if err != nil {
			mapLocationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toLocationResponse(loc))
	}
}

func toLocationResponse(loc *locationrepo.Location) locationResponse {
	return locationResponse{
		ID:                loc.ID,
		WorldID:           loc.WorldID,
		Slug:              loc.Slug,
		Name:              loc.Name,
		CountryCode:       loc.CountryCode,
		Region:            loc.Region,
		Kind:              loc.Kind,
		BoardIndex:        loc.BoardIndex,
		Price:             loc.Price,
		Rents:             loc.Rents,
		HouseCost:         loc.HouseCost,
		ColorGroup:        loc.ColorGroup,
		SpecialType:       loc.SpecialType,
		UtilityMultiplier: loc.UtilityMultiplier,
		TaxAmount:         loc.TaxAmount,
		PassBonus:         loc.PassBonus,
		Map: mapPoseResponse{
			X:     loc.Map.X,
			Z:     loc.Map.Z,
			Scale: loc.Map.Scale,
		},
		Assets:           assetsResponse{Icon: loc.Assets.Icon},
		HubID:            loc.HubID,
		EnterRadius:      loc.EnterRadius,
		Description:      loc.Description,
		Svgcities:        loc.Svgcities,
		SvgcitiesPath:    loc.SvgcitiesPath,
		SvgcitiesURL:     loc.SvgcitiesURL,
		Attribution:      loc.Attribution,
		Symbol:           loc.Symbol,
		SymbolStory:      loc.SymbolStory,
		About:            loc.About,
		AboutShort:       loc.AboutShort,
		Tags:             loc.Tags,
		LandmarkCategory: loc.LandmarkCategory,
	}
}

func mapLocationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, locationsvc.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Location not found")
	case errors.Is(err, locationsvc.ErrInvalidArgument):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		slog.Error("location request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}
