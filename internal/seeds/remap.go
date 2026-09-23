package seeds

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Classic boardIndex → (kind, specialType). specialType empty for property/rail/util.
var classicSlots = [40]struct {
	kind        string
	specialType string
}{
	0:  {"special", "go"},
	1:  {"property", ""},
	2:  {"special", "community_chest"},
	3:  {"property", ""},
	4:  {"special", "tax"},
	5:  {"railroad", ""},
	6:  {"property", ""},
	7:  {"special", "chance"},
	8:  {"property", ""},
	9:  {"property", ""},
	10: {"special", "jail"},
	11: {"property", ""},
	12: {"utility", ""},
	13: {"property", ""},
	14: {"property", ""},
	15: {"railroad", ""},
	16: {"property", ""},
	17: {"special", "community_chest"},
	18: {"property", ""},
	19: {"property", ""},
	20: {"special", "free_parking"},
	21: {"property", ""},
	22: {"special", "chance"},
	23: {"property", ""},
	24: {"property", ""},
	25: {"railroad", ""},
	26: {"property", ""},
	27: {"property", ""},
	28: {"utility", ""},
	29: {"property", ""},
	30: {"special", "go_to_jail"},
	31: {"property", ""},
	32: {"property", ""},
	33: {"special", "community_chest"},
	34: {"property", ""},
	35: {"railroad", ""},
	36: {"special", "chance"},
	37: {"property", ""},
	38: {"special", "tax"},
	39: {"property", ""},
}

// Classic Monopoly property color bands by boardIndex.
var classicColorGroup = map[int]string{
	1: "brown", 3: "brown",
	6: "lightBlue", 8: "lightBlue", 9: "lightBlue",
	11: "pink", 13: "pink", 14: "pink",
	16: "orange", 18: "orange", 19: "orange",
	21: "red", 23: "red", 24: "red",
	26: "yellow", 27: "yellow", 29: "yellow",
	31: "green", 32: "green", 34: "green",
	37: "darkBlue", 39: "darkBlue",
}

var trailingDigits = regexp.MustCompile(`\d+$`)

// RemapNotes are human-readable notes from a remap pass.
type RemapNotes []string

// RemapAllClassic40 rebuilds every world to a classic 40-space board.
// africa-1 docs are used as donors for missing specials in other worlds.
func RemapAllClassic40(docs []LocationDoc) ([]LocationDoc, RemapNotes, error) {
	byWorld := GroupByWorld(docs)
	africa, ok := byWorld["africa-1"]
	if !ok {
		return nil, nil, fmt.Errorf("africa-1 missing from seed")
	}

	var notes RemapNotes
	var rebuilt []LocationDoc
	for _, worldID := range SortedWorldIDs(byWorld) {
		out, worldNotes, err := remapWorld(worldID, byWorld[worldID], africa)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", worldID, err)
		}
		notes = append(notes, worldNotes...)
		rebuilt = append(rebuilt, out...)
	}
	return rebuilt, notes, nil
}

func remapWorld(worldID string, docs, africaDonor []LocationDoc) ([]LocationDoc, RemapNotes, error) {
	var notes RemapNotes
	props, leftovers := pickProperties(docs)
	if len(leftovers) >= 15 {
		notes = append(notes, fmt.Sprintf(
			"%s: %d leftover cities (≥15) — not creating -2 in this pass",
			worldID, len(leftovers),
		))
	} else if len(leftovers) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%s: parked %d leftover cities (<15, no -2)",
			worldID, len(leftovers),
		))
	}

	railsExisting := filterKind(docs, "railroad")
	sort.Slice(railsExisting, func(i, j int) bool {
		return railsExisting[i].BoardIndex < railsExisting[j].BoardIndex
	})
	utilsExisting := filterKind(docs, "utility")

	africaBySlug := map[string]LocationDoc{}
	for _, d := range africaDonor {
		africaBySlug[d.Slug] = d
	}

	goDoc := ensureSpecial(docs, worldID, specialOpts{
		slug: "go", specialType: "go", name: "GO", boardCode: "GO",
		icon:  "city-icons/generic/arrow-narrow-right.svg",
		donor: firstSpecial(docs, "go", africaBySlug["go"]),
	})
	jail := ensureSpecial(docs, worldID, specialOpts{
		slug: "jail", specialType: "jail", name: "Jail — Just Visiting", boardCode: "JAL",
		icon:  "city-icons/generic/prison.svg",
		donor: firstSpecial(docs, "jail", africaBySlug["jail"]),
	})
	parking := ensureSpecial(docs, worldID, specialOpts{
		slug: "free-parking", specialType: "free_parking", name: "Layover", boardCode: "P",
		icon:  "city-icons/generic/parking.svg",
		donor: firstSpecial(docs, "free_parking", africaBySlug["free-parking"]),
	})
	gotoJail := ensureSpecial(docs, worldID, specialOpts{
		slug: "go-to-jail", specialType: "go_to_jail", name: "Go to Jail", boardCode: "GTJ",
		icon:  "city-icons/generic/prison.svg",
		donor: firstSpecial(docs, "go_to_jail", africaBySlug["go-to-jail"]),
	})
	taxIncome := ensureSpecial(docs, worldID, specialOpts{
		slug: "tax-income", specialType: "tax", name: "Income Tax", boardCode: "TAX",
		icon:  "city-icons/generic/tax.svg",
		donor: firstSlugOrSpecial(docs, "tax-income", "tax", africaBySlug["tax-income"]),
	})
	taxLuxury := ensureSpecial(docs, worldID, specialOpts{
		slug: "tax-luxury", specialType: "tax", name: "Luxury Tax", boardCode: "LTX",
		icon:  "city-icons/generic/tax.svg",
		donor: firstSlug(docs, "tax-luxury", africaBySlug["tax-luxury"], taxIncome),
	})

	chances := make([]LocationDoc, 0, 3)
	for n := 1; n <= 3; n++ {
		slug := "chance"
		if n > 1 {
			slug = fmt.Sprintf("chance-%d", n)
		}
		chances = append(chances, ensureSpecial(docs, worldID, specialOpts{
			slug: slug, specialType: "chance", name: "Chance", boardCode: "CHA",
			icon:  "city-icons/generic/question-mark.svg",
			donor: firstSpecial(docs, "chance", africaBySlug["chance"]),
		}))
	}

	chests := make([]LocationDoc, 0, 3)
	for n := 1; n <= 3; n++ {
		slug := "community-chest"
		if n > 1 {
			slug = fmt.Sprintf("community-chest-%d", n)
		}
		chests = append(chests, ensureSpecial(docs, worldID, specialOpts{
			slug: slug, specialType: "community_chest", name: "Community Chest", boardCode: "CHE",
			icon:  "city-icons/generic/treasure-chest.svg",
			donor: firstSpecial(docs, "community_chest", africaBySlug["community-chest"]),
		}))
	}

	rails := make([]LocationDoc, 4)
	for n := 1; n <= 4; n++ {
		rails[n-1] = ensureRail(docs, worldID, n, railsExisting)
	}
	utilPower := ensureUtil(docs, worldID, "power", utilsExisting)
	utilWater := ensureUtil(docs, worldID, "water", utilsExisting)

	propI, chanceI, chestI, railI := 0, 0, 0, 0
	out := make([]LocationDoc, 0, 40)

	for idx := 0; idx < 40; idx++ {
		slot := classicSlots[idx]
		var doc LocationDoc
		switch {
		case slot.kind == "property":
			doc = props[propI]
			propI++
		case slot.kind == "railroad":
			doc = rails[railI]
			railI++
		case slot.kind == "utility":
			if idx == 12 {
				doc = utilPower
			} else {
				doc = utilWater
			}
		case slot.specialType == "go":
			doc = goDoc
		case slot.specialType == "jail":
			doc = jail
		case slot.specialType == "free_parking":
			doc = parking
		case slot.specialType == "go_to_jail":
			doc = gotoJail
		case slot.specialType == "tax" && idx == 4:
			doc = taxIncome
		case slot.specialType == "tax" && idx == 38:
			doc = taxLuxury
		case slot.specialType == "chance":
			doc = chances[chanceI]
			chanceI++
		case slot.specialType == "community_chest":
			doc = chests[chestI]
			chestI++
		default:
			return nil, nil, fmt.Errorf("unhandled slot %d %s/%s", idx, slot.kind, slot.specialType)
		}

		doc = Clone(doc)
		doc.WorldID = worldID
		doc.BoardIndex = idx
		if slot.kind == "property" {
			if cg, ok := classicColorGroup[idx]; ok {
				doc.ColorGroup = strPtr(cg)
			}
		} else {
			doc.ColorGroup = nil
		}
		if doc.Rents == nil {
			doc.Rents = []int{}
		}
		out = append(out, doc)
	}

	uniqueBoardCodes(out)
	if err := validateClassicWorld(out); err != nil {
		return nil, nil, err
	}
	return out, notes, nil
}

type specialOpts struct {
	slug, specialType, name, boardCode, icon string
	donor                                    *LocationDoc
}

func ensureSpecial(docs []LocationDoc, world string, opts specialOpts) LocationDoc {
	existing := findSlug(docs, opts.slug)
	if existing == nil {
		existing = findBySpecial(docs, opts.specialType)
	}
	var base LocationDoc
	switch {
	case existing != nil && specialType(*existing) == opts.specialType:
		base = Clone(*existing)
	case opts.donor != nil:
		base = Clone(*opts.donor)
	default:
		base = LocationDoc{
			WorldID:     world,
			Kind:        "special",
			Price:       0,
			Map:         MapPose{Scale: 1},
			EnterRadius: 1.2,
			HubID:       fmt.Sprintf("hub:%s:%s", world, opts.slug),
			Assets:      Assets{Icon: opts.icon},
		}
	}
	base.WorldID = world
	base.Slug = opts.slug
	base.Kind = "special"
	base.SpecialType = strPtr(opts.specialType)
	base.Name = opts.name
	base.BoardCode = opts.boardCode
	base.Assets = Assets{Icon: opts.icon}
	base.HubID = fmt.Sprintf("hub:%s:%s", world, opts.slug)
	base.Price = 0
	base.Description = opts.name
	if opts.specialType == "free_parking" {
		base.Name = "Layover"
		base.Description = "Layover"
		base.AboutShort = "Layover — free rest stop"
	}
	return base
}

func ensureRail(docs []LocationDoc, world string, n int, donorPool []LocationDoc) LocationDoc {
	slug := fmt.Sprintf("rail-%d", n)
	if existing := findSlug(docs, slug); existing != nil {
		return Clone(*existing)
	}
	var template LocationDoc
	if len(donorPool) > 0 {
		idx := n - 1
		if idx >= len(donorPool) {
			idx = len(donorPool) - 1
		}
		template = Clone(donorPool[idx])
	} else {
		template = LocationDoc{
			Kind:        "railroad",
			Price:       200,
			Map:         MapPose{Scale: 1},
			EnterRadius: 1.5,
			Assets:      Assets{Icon: "city-icons/generic/plane-tilt.svg"},
		}
	}
	template.WorldID = world
	template.Slug = slug
	template.Kind = "railroad"
	if template.Name == "" {
		template.Name = fmt.Sprintf("Air Hub %d", n)
	}
	if template.BoardCode == "" {
		template.BoardCode = fmt.Sprintf("R%d", n)
	}
	template.Assets = Assets{Icon: "city-icons/generic/plane-tilt.svg"}
	template.HubID = fmt.Sprintf("hub:%s:%s", world, slug)
	template.SpecialType = nil
	return template
}

func ensureUtil(docs []LocationDoc, world, which string, donorPool []LocationDoc) LocationDoc {
	slug := fmt.Sprintf("utility-%s", which)
	if existing := findSlug(docs, slug); existing != nil {
		return Clone(*existing)
	}
	var template *LocationDoc
	for i := range donorPool {
		d := &donorPool[i]
		slugL := strings.ToLower(d.Slug)
		nameL := strings.ToLower(d.Name)
		if strings.Contains(slugL, which) || strings.Contains(nameL, which) {
			template = d
			break
		}
	}
	if template == nil && len(donorPool) > 0 {
		template = &donorPool[0]
	}
	var base LocationDoc
	if template != nil {
		base = Clone(*template)
	} else {
		base = LocationDoc{
			Kind:        "utility",
			Price:       150,
			Map:         MapPose{Scale: 1},
			EnterRadius: 1.5,
		}
	}
	base.WorldID = world
	base.Slug = slug
	base.Kind = "utility"
	base.HubID = fmt.Sprintf("hub:%s:%s", world, slug)
	base.SpecialType = nil
	nameL := strings.ToLower(base.Name)
	if which == "power" {
		if !strings.Contains(nameL, "power") && !strings.Contains(nameL, "electric") {
			base.Name = fmt.Sprintf("%s Power Grid", world)
		}
		base.BoardCode = "ELC"
		base.Assets = Assets{Icon: "city-icons/generic/bolt.svg"}
	} else {
		if !strings.Contains(nameL, "water") {
			base.Name = fmt.Sprintf("%s Water Works", world)
		}
		base.BoardCode = "WTR"
		base.Assets = Assets{Icon: "city-icons/generic/droplet.svg"}
	}
	return base
}

func pickProperties(docs []LocationDoc) (keep, leftovers []LocationDoc) {
	props := filterKind(docs, "property")
	sort.Slice(props, func(i, j int) bool {
		return props[i].BoardIndex < props[j].BoardIndex
	})
	if len(props) >= 22 {
		for i := 0; i < 22; i++ {
			keep = append(keep, Clone(props[i]))
		}
		for i := 22; i < len(props); i++ {
			leftovers = append(leftovers, Clone(props[i]))
		}
		return keep, leftovers
	}
	if len(props) == 0 {
		return nil, nil
	}
	for _, p := range props {
		keep = append(keep, Clone(p))
	}
	i := 0
	for len(keep) < 22 {
		src := props[i%len(props)]
		pad := Clone(src)
		n := (len(keep) / len(props)) + 1
		pad.Slug = fmt.Sprintf("%s-r%d", src.Slug, n)
		pad.HubID = fmt.Sprintf("hub:%s:%s", src.WorldID, pad.Slug)
		keep = append(keep, pad)
		i++
	}
	return keep, nil
}

func uniqueBoardCodes(docs []LocationDoc) {
	for i := range docs {
		st := specialType(docs[i])
		if st == "chance" || st == "community_chest" {
			docs[i].BoardCode = ""
		}
	}

	used := map[string]struct{}{}
	for _, d := range docs {
		if d.BoardCode != "" {
			used[d.BoardCode] = struct{}{}
		}
	}

	take := func(preferred string) string {
		if _, ok := used[preferred]; !ok {
			used[preferred] = struct{}{}
			return preferred
		}
		base := trailingDigits.ReplaceAllString(preferred, "")
		if base == "" {
			base = preferred
		}
		n := 2
		for {
			code := fmt.Sprintf("%s%d", base, n)
			if _, ok := used[code]; !ok {
				used[code] = struct{}{}
				return code
			}
			n++
		}
	}

	var chances, chests []int
	for i, d := range docs {
		switch specialType(d) {
		case "chance":
			chances = append(chances, i)
		case "community_chest":
			chests = append(chests, i)
		}
	}
	sort.Slice(chances, func(i, j int) bool {
		return docs[chances[i]].BoardIndex < docs[chances[j]].BoardIndex
	})
	sort.Slice(chests, func(i, j int) bool {
		return docs[chests[i]].BoardIndex < docs[chests[j]].BoardIndex
	})
	for _, idx := range chances {
		docs[idx].BoardCode = "CHA"
	}
	for _, idx := range chests {
		docs[idx].BoardCode = "CHE"
	}

	for i := range docs {
		if docs[i].BoardCode == "" {
			docs[i].BoardCode = take("X")
		}
	}

	seen := map[string]struct{}{}
	for i := range docs {
		st := specialType(docs[i])
		if st == "chance" || st == "community_chest" {
			continue
		}
		code := docs[i].BoardCode
		if code == "" {
			code = "X"
		}
		if _, ok := seen[code]; ok {
			docs[i].BoardCode = take(code)
		} else {
			seen[code] = struct{}{}
			used[code] = struct{}{}
		}
	}
}

func validateClassicWorld(docs []LocationDoc) error {
	if len(docs) != 40 {
		return fmt.Errorf("want 40 spaces, got %d", len(docs))
	}
	slugs := map[string]struct{}{}
	var chanceIdx, chestIdx []int
	for _, d := range docs {
		if _, ok := slugs[d.Slug]; ok {
			return fmt.Errorf("duplicate slug %q", d.Slug)
		}
		slugs[d.Slug] = struct{}{}
		switch specialType(d) {
		case "chance":
			chanceIdx = append(chanceIdx, d.BoardIndex)
			if d.BoardCode != "CHA" {
				return fmt.Errorf("chance %q boardCode %q want CHA", d.Slug, d.BoardCode)
			}
		case "community_chest":
			chestIdx = append(chestIdx, d.BoardIndex)
			if d.BoardCode != "CHE" {
				return fmt.Errorf("chest %q boardCode %q want CHE", d.Slug, d.BoardCode)
			}
		}
	}
	if err := validateBoardCodes(docs); err != nil {
		return err
	}
	sort.Ints(chanceIdx)
	sort.Ints(chestIdx)
	if fmt.Sprint(chanceIdx) != "[7 22 36]" {
		return fmt.Errorf("chance indices %v want [7 22 36]", chanceIdx)
	}
	if fmt.Sprint(chestIdx) != "[2 17 33]" {
		return fmt.Errorf("chest indices %v want [2 17 33]", chestIdx)
	}
	return nil
}

func filterKind(docs []LocationDoc, kind string) []LocationDoc {
	var out []LocationDoc
	for _, d := range docs {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	return out
}

func findSlug(docs []LocationDoc, slug string) *LocationDoc {
	for i := range docs {
		if docs[i].Slug == slug {
			return &docs[i]
		}
	}
	return nil
}

func findBySpecial(docs []LocationDoc, st string) *LocationDoc {
	for i := range docs {
		if specialType(docs[i]) == st {
			return &docs[i]
		}
	}
	return nil
}

func firstSpecial(docs []LocationDoc, st string, fallback LocationDoc) *LocationDoc {
	if d := findBySpecial(docs, st); d != nil {
		return d
	}
	if fallback.Slug != "" || fallback.Kind != "" {
		c := Clone(fallback)
		return &c
	}
	return nil
}

func firstSlug(docs []LocationDoc, slug string, fallbacks ...LocationDoc) *LocationDoc {
	if d := findSlug(docs, slug); d != nil {
		return d
	}
	for _, f := range fallbacks {
		if f.Slug != "" || f.Kind != "" {
			c := Clone(f)
			return &c
		}
	}
	return nil
}

func firstSlugOrSpecial(docs []LocationDoc, slug, st string, fallback LocationDoc) *LocationDoc {
	if d := findSlug(docs, slug); d != nil {
		return d
	}
	if d := findBySpecial(docs, st); d != nil {
		return d
	}
	if fallback.Slug != "" || fallback.Kind != "" {
		c := Clone(fallback)
		return &c
	}
	return nil
}
