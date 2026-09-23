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

	rebuilt, notes, err := seeds.RemapAllClassic40(docs)
	if err != nil {
		log.Fatalf("remap: %v", err)
	}

	for _, n := range notes {
		fmt.Println(n)
	}

	byWorld := seeds.GroupByWorld(rebuilt)
	fmt.Println("\nSummary:")
	for _, w := range seeds.SortedWorldIDs(byWorld) {
		props := 0
		for _, d := range byWorld[w] {
			if d.Kind == "property" {
				props++
			}
		}
		fmt.Printf("  %s: %d spaces, props=%d\n", w, len(byWorld[w]), props)
	}

	if err := seeds.WriteFile(*seedPath, rebuilt); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Fprintf(os.Stderr, "\nwrote %s (%d docs)\n", *seedPath, len(rebuilt))
}
