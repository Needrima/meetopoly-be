package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"meetopoly-be/internal/seeds"
)

func main() {
	seedPath := flag.String("file", "seeds/locations.json", "path to locations JSON array")
	flag.Parse()

	docs, err := seeds.LoadFile(*seedPath)
	if err != nil {
		log.Fatalf("load: %v", err)
	}

	out, result, err := seeds.PolishAirportsIcons(docs)
	if err != nil {
		log.Fatalf("polish: %v", err)
	}

	if err := seeds.WriteFile(*seedPath, out); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Fprintf(os.Stderr, "updated %s\n", *seedPath)

	for _, worldID := range seeds.SortedWorldIDs(seeds.GroupByWorld(out)) {
		lines, ok := result.AirportsByWorld[worldID]
		if !ok || len(lines) == 0 {
			continue
		}
		fmt.Println(worldID)
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
	}
}
