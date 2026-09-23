package seeds

import (
	"fmt"
	"hash/fnv"
	"sort"
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

// PolishAirportsIcons sets prison/tax icons, named airports, and city boardCodes.
//
// Board-code policy (locked):
//   - Chance / Chest always CHA / CHE (duplicates OK).
//   - Cities may share a code with an airport (IATA); plane icon + Details disambiguate.
//   - City–city collisions use letter alternates from the name (LOS / LAN), never LA2-style digits.
//   - When several cities want the same preferred code, a stable hash of world+slug picks order.
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
		case "chance":
			out[i].BoardCode = "CHA"
		case "community_chest":
			out[i].BoardCode = "CHE"
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

	if err := assignPropertyBoardCodes(out); err != nil {
		return nil, PolishResult{}, err
	}
	assignAnonymousRailCodes(out)

	if err := validateBoardCodes(out); err != nil {
		return nil, PolishResult{}, err
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

// assignPropertyBoardCodes sets unique-among-cities codes using letter alternates.
// Airport IATA codes are ignored for collisions (city may match airport).
func assignPropertyBoardCodes(docs []LocationDoc) error {
	byWorld := map[string][]int{}
	for i := range docs {
		if docs[i].Kind == "property" {
			byWorld[docs[i].WorldID] = append(byWorld[docs[i].WorldID], i)
		}
	}

	for world, idxs := range byWorld {
		// Exclusive pool: utilities + non-CHA/CHE specials (not airports, not other cities yet).
		// Also reserve CHA/CHE so cities never steal Chance/Chest labels.
		used := map[string]struct{}{
			"CHA": {},
			"CHE": {},
		}
		for i := range docs {
			if docs[i].WorldID != world {
				continue
			}
			st := specialType(docs[i])
			if st == "chance" || st == "community_chest" {
				continue
			}
			if docs[i].Kind == "railroad" || docs[i].Kind == "property" {
				continue
			}
			if docs[i].BoardCode != "" {
				used[docs[i].BoardCode] = struct{}{}
			}
		}

		type claim struct {
			idx       int
			preferred string
		}
		claims := make([]claim, 0, len(idxs))
		for _, idx := range idxs {
			pref := preferredCityCode(docs[idx].Name)
			claims = append(claims, claim{idx: idx, preferred: pref})
		}

		// Stable “random”: sort by hash(world|slug) then boardIndex.
		sort.Slice(claims, func(a, b int) bool {
			ha := slugHash(world, docs[claims[a].idx].Slug)
			hb := slugHash(world, docs[claims[b].idx].Slug)
			if ha != hb {
				return ha < hb
			}
			return docs[claims[a].idx].BoardIndex < docs[claims[b].idx].BoardIndex
		})

		// Group by preferred code; first in sorted order wins the preferred.
		seenPref := map[string]int{} // preferred → how many already assigned that pref as base
		for _, c := range claims {
			code := c.preferred
			if _, taken := used[code]; taken || seenPref[c.preferred] > 0 {
				code = nextLetterCode(docs[c.idx].Name, c.preferred, used)
			}
			docs[c.idx].BoardCode = code
			used[code] = struct{}{}
			seenPref[c.preferred]++
		}
	}
	return nil
}

// assignAnonymousRailCodes cleans padded/repeat rails that are not in the airport map
// (no numeric suffixes like SI2 / CUN3).
func assignAnonymousRailCodes(docs []LocationDoc) {
	byWorld := map[string][]int{}
	for i := range docs {
		byWorld[docs[i].WorldID] = append(byWorld[docs[i].WorldID], i)
	}
	for world, idxs := range byWorld {
		worldAirports := airports[world]
		used := map[string]struct{}{
			"CHA": {},
			"CHE": {},
		}
		for _, i := range idxs {
			d := docs[i]
			st := specialType(d)
			if st == "chance" || st == "community_chest" {
				continue
			}
			// Reserve known airport IATA + all non-anonymous codes.
			if d.Kind == "railroad" && worldAirports != nil {
				if _, named := worldAirports[d.Slug]; named {
					if d.BoardCode != "" {
						used[d.BoardCode] = struct{}{}
					}
					continue
				}
			}
			if d.Kind == "railroad" {
				continue // assign below
			}
			if d.BoardCode != "" {
				used[d.BoardCode] = struct{}{}
			}
		}
		for _, i := range idxs {
			d := &docs[i]
			if d.Kind != "railroad" {
				continue
			}
			if worldAirports != nil {
				if _, named := worldAirports[d.Slug]; named {
					continue
				}
			}
			pref := preferredCityCode(d.Name)
			if pref == "XXX" || pref == "" {
				pref = "HUB"
			}
			code := pref
			if _, ok := used[code]; ok {
				code = nextLetterCode(d.Name, pref, used)
			}
			d.BoardCode = code
			used[code] = struct{}{}
		}
	}
}

func preferredCityCode(name string) string {
	letters := lettersOnly(name)
	switch {
	case len(letters) >= 3:
		return letters[:3]
	case len(letters) == 2:
		return letters + "X"
	case len(letters) == 1:
		return letters + "XX"
	default:
		return "XXX"
	}
}

// nextLetterCode picks another 3-letter code from the city name (order-preserving
// subsequences), then alphabet padding — never numeric suffixes.
func nextLetterCode(name, preferred string, used map[string]struct{}) string {
	if _, ok := used[preferred]; !ok {
		return preferred
	}
	letters := lettersOnly(name)
	for _, cand := range letterTriples(letters) {
		if _, ok := used[cand]; !ok {
			return cand
		}
	}
	// 2-letter stems + A–Z
	stem := preferred
	if len(stem) > 2 {
		stem = stem[:2]
	}
	for r := 'A'; r <= 'Z'; r++ {
		cand := stem + string(r)
		if _, ok := used[cand]; !ok {
			return cand
		}
	}
	for n := 0; n < 1000; n++ {
		cand := fmt.Sprintf("X%02d", n)
		if _, ok := used[cand]; !ok {
			return cand
		}
	}
	return "ZZZ"
}

func letterTriples(letters string) []string {
	var out []string
	seen := map[string]struct{}{}
	n := len(letters)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			for k := j + 1; k < n; k++ {
				cand := string([]byte{letters[i], letters[j], letters[k]})
				if _, ok := seen[cand]; ok {
					continue
				}
				seen[cand] = struct{}{}
				out = append(out, cand)
			}
		}
	}
	return out
}

func slugHash(world, slug string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(world))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(slug))
	return h.Sum64()
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

// validateBoardCodes allows CHA/CHE duplicates and property+airport shared codes.
func validateBoardCodes(docs []LocationDoc) error {
	type key struct {
		world string
		code  string
	}
	groups := map[key][]LocationDoc{}
	for _, d := range docs {
		k := key{world: d.WorldID, code: d.BoardCode}
		groups[k] = append(groups[k], d)
	}
	for k, group := range groups {
		if len(group) <= 1 {
			continue
		}
		if boardCodeDupOK(group) {
			continue
		}
		names := make([]string, 0, len(group))
		for _, d := range group {
			names = append(names, fmt.Sprintf("%s/%s", d.Kind, d.Slug))
		}
		return fmt.Errorf("duplicate boardCode %q in %s: %s", k.code, k.world, strings.Join(names, ", "))
	}
	return nil
}

func boardCodeDupOK(group []LocationDoc) bool {
	allChance := true
	allChest := true
	onlyPropAndRail := true
	for _, d := range group {
		st := specialType(d)
		if st != "chance" {
			allChance = false
		}
		if st != "community_chest" {
			allChest = false
		}
		if d.Kind != "property" && d.Kind != "railroad" {
			onlyPropAndRail = false
		}
	}
	if allChance || allChest {
		return true
	}
	if !onlyPropAndRail {
		return false
	}
	hasProp, hasRail := false, false
	for _, d := range group {
		if d.Kind == "property" {
			hasProp = true
		}
		if d.Kind == "railroad" {
			hasRail = true
		}
	}
	// Shared IATA-style label between city and airport(s) is OK.
	// Two properties still not OK (onlyPropAndRail && !hasRail would be two props).
	propCount, railCount := 0, 0
	for _, d := range group {
		switch d.Kind {
		case "property":
			propCount++
		case "railroad":
			railCount++
		}
	}
	return hasProp && hasRail && propCount <= 1
}
