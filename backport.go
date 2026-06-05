package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

// runBackportCLI parses the `backport` subcommand:
//
//	printodo-api backport [--dry-run] [--start-months N] [--end-months N] <notes-file>
//
// The notes file is one note per line. File order is treated as oldest -> newest
// (top line oldest, bottom line newest, matching receipt order), and notes are
// spread randomly over the window [now-start-months, now-end-months]. Defaults
// cover the last 2 years; pass e.g. --start-months 12 --end-months 0 for the
// recent half and --start-months 24 --end-months 12 for an older half.
func runBackportCLI(args []string) {
	fs := flag.NewFlagSet("backport", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "preview the date mapping without writing or calling the AI")
	startM := fs.Int("start-months", 24, "oldest bound of the date window, in months ago")
	endM := fs.Int("end-months", 0, "newest bound of the date window, in months ago")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	path := fs.Arg(0)
	if path == "" {
		log.Fatal("usage: printodo-api backport [--dry-run] [--start-months N] [--end-months N] <notes-file>")
	}

	texts, times, err := planBackport(path, *startM, *endM)
	if err != nil {
		log.Fatal(err)
	}

	if *dry {
		previewBackport(texts, times, *startM, *endM)
		return
	}

	api := openAPI()
	api.runBackport(texts, times)
}

// planBackport reads the notes and assigns each a date within the window,
// preserving file order as chronological order (top oldest, bottom newest).
func planBackport(path string, startMonths, endMonths int) ([]string, []time.Time, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var texts []string
	for _, line := range strings.Split(string(raw), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			texts = append(texts, t)
		}
	}
	if len(texts) == 0 {
		return nil, nil, fmt.Errorf("no notes found in %s", path)
	}

	now := time.Now()
	start := now.AddDate(0, -startMonths, 0)
	end := now.AddDate(0, -endMonths, 0)
	span := int64(end.Sub(start))
	if span <= 0 {
		return nil, nil, fmt.Errorf("invalid window: start-months (%d) must be greater than end-months (%d)", startMonths, endMonths)
	}

	times := make([]time.Time, len(texts))
	for i := range times {
		times[i] = start.Add(time.Duration(rand.Int63n(span)))
	}
	// Ascending, then mapped to file order so the first line is oldest and the
	// last line is newest.
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	return texts, times, nil
}

func previewBackport(texts []string, times []time.Time, startMonths, endMonths int) {
	fmt.Printf("Backport preview: %d notes spread over %d..%d months ago (%s -> %s)\n\n",
		len(texts), startMonths, endMonths,
		times[0].Format("2006-01-02"), times[len(times)-1].Format("2006-01-02"))

	show := func(label string, idxs []int) {
		fmt.Println(label)
		for _, i := range idxs {
			fmt.Printf("  %s  %s\n", times[i].Format("2006-01-02"), texts[i])
		}
	}
	n := len(texts)
	head := []int{}
	for i := 0; i < 5 && i < n; i++ {
		head = append(head, i)
	}
	tail := []int{}
	for i := n - 5; i < n; i++ {
		if i >= 0 {
			tail = append(tail, i)
		}
	}
	show("Oldest (top of file):", head)
	fmt.Println("  ...")
	show("Newest (bottom of file):", tail)
}

// runBackport inserts the planned notes: each as already-printed history (so it
// never re-queues to the printer), with its assigned date, then AI-classified
// (categories + brand/product links) when ANTHROPIC_API_KEY is set.
func (api *API) runBackport(texts []string, times []time.Time) {
	apiKey := getEnv("ANTHROPIC_API_KEY", "")
	if apiKey == "" {
		log.Println("[backport] ANTHROPIC_API_KEY not set — importing without AI classification")
	}

	imported, classified := 0, 0
	for i, text := range texts {
		ts := times[i]
		item := &Item{Text: text, CreatedAt: ts, PrintedAt: &ts}
		if err := api.db.Create(item).Error; err != nil {
			log.Printf("[backport] insert failed for %q: %v", text, err)
			continue
		}
		imported++

		var cats []string
		var links []Link
		if apiKey != "" {
			known, _ := api.AllCategoryNames()
			var err error
			cats, links, err = requestAI(apiKey, text, known)
			if err != nil {
				log.Printf("[backport] AI failed for %q: %v", text, err)
			}
		}
		if len(cats) > 0 || len(links) > 0 {
			if e := api.SetAIResults(item.ID, cats, links); e != nil {
				log.Printf("[backport] saving AI results failed for %q: %v", text, e)
			}
		}
		if len(cats) > 0 {
			if e := api.ClassifyItem(item.ID, cats); e != nil {
				log.Printf("[backport] classify failed for %q: %v", text, e)
			} else {
				classified++
			}
		}
		log.Printf("[backport] %d/%d  %s  %-40.40q  cats=%v", i+1, len(texts), ts.Format("2006-01-02"), text, cats)
	}

	log.Printf("[backport] done: imported %d notes (%d auto-classified, %d left for the worklist)",
		imported, classified, imported-classified)
}
