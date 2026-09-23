package game

import (
	"context"

	tablesvc "meetopoly-be/internal/services/table"
)

// TableBridge adapts game.Service to tablesvc.GameStarter.
type TableBridge struct {
	Games Service
}

// StartFromTable implements tablesvc.GameStarter.
func (b TableBridge) StartFromTable(ctx context.Context, tableID, worldID string, seats []tablesvc.SeatView) (string, error) {
	inputs := make([]SeatInput, 0, len(seats))
	for _, s := range seats {
		if s.UserID == nil || *s.UserID == "" {
			continue
		}
		name := ""
		if s.Username != nil {
			name = *s.Username
		}
		inputs = append(inputs, SeatInput{
			UserID:    *s.UserID,
			Username:  name,
			SeatIndex: s.SeatIndex,
		})
	}
	view, err := b.Games.CreateFromSeats(ctx, tableID, worldID, inputs)
	if err != nil {
		return "", err
	}
	return view.ID, nil
}
