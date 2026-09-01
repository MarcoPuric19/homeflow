// Command mcimport carries the history in Austausch.md over into the ticket
// board. It is safe to run twice: tickets are matched by title and existing
// ones are skipped rather than duplicated.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"missioncontrol/internal/tickets"
)

func main() {
	source := flag.String("source", "Austausch.md", "Datei, die übernommen wird")
	dir := flag.String("tickets", filepath.Join("missioncontrol", "tickets"), "Ticket-Verzeichnis")
	author := flag.String("author", "Claude Code", "wer den Import ausführt")
	homeflow := flag.String("homeflow-version", "", "aktuelle HomeFlow-Version")
	mission := flag.String("mission-version", "", "aktuelle Mission-Control-Version")
	dry := flag.Bool("dry-run", false, "nur zeigen, was entstehen würde")
	flag.Parse()

	data, err := os.ReadFile(*source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	target := *dir
	if *dry {
		target, err = os.MkdirTemp("", "mcimport-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer os.RemoveAll(target)
	}
	store, err := tickets.NewStore(target, map[string]string{
		tickets.ProjectHomeFlow: *homeflow,
		tickets.ProjectMission:  *mission,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	result, err := store.Import(string(data), *author)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	counts := map[string]int{}
	for _, ticket := range result.Created {
		counts[ticket.Type]++
		fmt.Printf("  %-8s %-12s %s\n", ticket.Key, ticket.Type, ticket.Title)
	}
	fmt.Printf("\n%d Tickets angelegt", len(result.Created))
	for _, kind := range tickets.Types() {
		if counts[kind] > 0 {
			fmt.Printf(" · %s: %d", kind, counts[kind])
		}
	}
	fmt.Printf("\n%d übersprungen (Titel existiert bereits)\n", len(result.Skipped))
	if *dry {
		fmt.Println("Probelauf - nichts geschrieben.")
	}
}
