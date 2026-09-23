package seeds

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	iconPrison = "city-icons/generic/prison.svg"
	iconTax    = "city-icons/generic/tax.svg"
	iconPlane  = "city-icons/generic/plane-tilt.svg"
)

// airportEntry is name + IATA-ish boardCode for a rail slug.
type airportEntry struct {
	name string
	code string
}

// Airports keyed by worldId → rail slug.
var airports = map[string]map[string]airportEntry{
	"africa-1": {
		"rail-1": {"Cairo International Airport", "CAI"},
		"rail-2": {"Jomo Kenyatta International Airport", "NBO"},
		"rail-3": {"O.R. Tambo International Airport", "JNB"},
		"rail-4": {"Murtala Muhammed International Airport", "MM"},
	},
	"europe-1": {
		"rail-1": {"Heathrow Airport", "LHR"},
		"rail-2": {"Charles de Gaulle Airport", "CDG"},
		"rail-3": {"Adolfo Suárez Madrid-Barajas Airport", "MAD"},
		"rail-4": {"Amsterdam Schiphol Airport", "AMS"},
	},
	"europe-2": {
		"rail-1": {"Frankfurt Airport", "FRA"},
		"rail-2": {"Munich Airport", "MUC"},
		"rail-3": {"Zurich Airport", "ZRH"},
		"rail-4": {"Vienna International Airport", "VIE"},
	},
	"europe-3": {
		"rail-1": {"Leonardo da Vinci–Fiumicino Airport", "FCO"},
		"rail-2": {"Milan Malpensa Airport", "MXP"},
		"rail-3": {"Athens International Airport", "ATH"},
		"rail-4": {"Lisbon Humberto Delgado Airport", "LIS"},
	},
	"europe-4": {
		"rail-1": {"Stockholm Arlanda Airport", "ARN"},
		"rail-2": {"Oslo Gardermoen Airport", "OSL"},
		"rail-3": {"Copenhagen Airport", "CPH"},
		"rail-4": {"Helsinki Airport", "HEL"},
	},
	"europe-5": {
		"rail-1": {"Warsaw Chopin Airport", "WAW"},
		"rail-2": {"Václav Havel Airport Prague", "PRG"},
		"rail-3": {"Budapest Ferenc Liszt Airport", "BUD"},
		"rail-4": {"Bucharest Henri Coandă Airport", "OTP"},
	},
	"asia-1": {
		"rail-1": {"Tokyo Narita International Airport", "NRT"},
		"rail-2": {"Beijing Capital International Airport", "PEK"},
		"rail-4": {"Singapore Changi Airport", "SIN"},
	},
	"asia-2": {
		"rail-1": {"Seoul Incheon International Airport", "ICN"},
		"rail-2": {"Hong Kong International Airport", "HKG"},
		"rail-4": {"Indira Gandhi International Airport", "DEL"},
	},
	"north-america-1": {
		"rail-1": {"John F. Kennedy International Airport", "JFK"},
		"rail-2": {"O'Hare International Airport", "ORD"},
		"rail-3": {"Hartsfield–Jackson Atlanta International Airport", "ATL"},
		"rail-4": {"Los Angeles International Airport", "LAX"},
	},
	"south-america-1": {
		"rail-1": {"São Paulo–Guarulhos International Airport", "GRU"},
		"rail-2": {"Ministro Pistarini International Airport", "EZE"},
		"rail-3": {"El Dorado International Airport", "BOG"},
		"rail-4": {"Arturo Merino Benítez Airport", "SCL"},
	},
	"middle-east-1": {
		"rail-1": {"Dubai International Airport", "DXB"},
		"rail-2": {"Hamad International Airport", "DOH"},
		"rail-3": {"Istanbul Airport", "IST"},
		"rail-4": {"Abu Dhabi International Airport", "AUH"},
	},
	"oceania-1": {
		"rail-1": {"Sydney Kingsford Smith Airport", "SYD"},
		"rail-3": {"Auckland Airport", "AKL"},
	},
	"central-america-1": {
		"rail-1": {"Tocumen International Airport", "PTY"},
		"rail-3": {"Cancún International Airport", "CUN"},
	},
}

// PolishResult summarizes airport lines after polish.
type PolishResult struct {
	AirportsByWorld map[string][]string
}

// PolishAirportsIcons sets prison/tax icons and named airports + unique boardCodes.
// Document order is preserved.
func PolishAirportsIcons(docs []LocationDoc) ([]LocationDoc, PolishResult, error) {
	out := make([]LocationDoc, len(docs))
	copy(out, docs)

	for i := range out {
		st := specialType(out[i])
		switch st {
		case "jail", "go_to_jail":
			out[i].Assets = Assets{Icon: iconPrison}
		case "tax":
			out[i].Assets = Assets{Icon: iconTax}
		}

		world := out[i].WorldID
		if out[i].Kind != "railroad" {
			continue
		}
		worldAirports, ok := airports[world]
		if !ok {
			continue
		}
		ap, ok := worldAirports[out[i].Slug]
		if !ok {
			continue
		}
		out[i].Name = ap.name
		out[i].Description = ap.name
		out[i].AboutShort = fmt.Sprintf("%s (%s)", ap.name, ap.code)
		out[i].BoardCode = ap.code
		out[i].Assets = Assets{Icon: iconPlane}
	}

	// Resolve boardCode collisions within each world (airports win; bump others).
	usedByWorld := map[string]map[string]struct{}{}
	reservedByWorld := map[string]map[string]struct{}{}
	for i := range out {
		world := out[i].WorldID
		if reservedByWorld[world] == nil {
			reservedByWorld[world] = map[string]struct{}{}
		}
		if out[i].Kind == "railroad" {
			if worldAirports, ok := airports[world]; ok {
				if _, ok := worldAirports[out[i].Slug]; ok {
					reservedByWorld[world][out[i].BoardCode] = struct{}{}
				}
			}
		}
	}
	for world, reserved := range reservedByWorld {
		used := map[string]struct{}{}
		for c := range reserved {
			used[c] = struct{}{}
		}
		usedByWorld[world] = used
	}

	for i := range out {
		world := out[i].WorldID
		used := usedByWorld[world]
		if used == nil {
			used = map[string]struct{}{}
			usedByWorld[world] = used
		}
		if out[i].Kind == "railroad" {
			if worldAirports, ok := airports[world]; ok {
				if _, ok := worldAirports[out[i].Slug]; ok {
					continue
				}
			}
		}
		code := out[i].BoardCode
		if code == "" {
			code = lettersOnly(out[i].Name)
			if len(code) > 3 {
				code = code[:3]
			}
			if code == "" {
				code = "XXX"
			}
		}
		if _, ok := used[code]; ok {
			code = nextCode(code, used)
			out[i].BoardCode = code
		}
		used[code] = struct{}{}
	}

	counts := map[string]map[string]int{}
	for _, loc := range out {
		if counts[loc.WorldID] == nil {
			counts[loc.WorldID] = map[string]int{}
		}
		counts[loc.WorldID][loc.BoardCode]++
	}
	for world, codes := range counts {
		for code, n := range codes {
			if n > 1 {
				return nil, PolishResult{}, fmt.Errorf("dup boardCode %q in %s", code, world)
			}
		}
	}

	result := PolishResult{AirportsByWorld: map[string][]string{}}
	for _, loc := range out {
		if loc.Kind != "railroad" {
			continue
		}
		line := fmt.Sprintf("%-8s %-4s %s", loc.Slug, loc.BoardCode, loc.Name)
		result.AirportsByWorld[loc.WorldID] = append(result.AirportsByWorld[loc.WorldID], line)
	}

	return out, result, nil
}

func lettersOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

func nextCode(base string, used map[string]struct{}) string {
	if _, ok := used[base]; !ok {
		return base
	}
	letters := lettersOnly(base)
	if letters == "" {
		letters = "X"
	}
	stem := letters
	if len(stem) >= 2 {
		stem = stem[:2]
	}
	n := 2
	for {
		code := fmt.Sprintf("%s%d", stem, n)
		if _, ok := used[code]; !ok {
			return code
		}
		n++
	}
}
