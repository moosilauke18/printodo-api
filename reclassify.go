package main

import (
	"flag"
	"log"
)

// runReclassifyCLI parses the `reclassify` subcommand:
//
//	printodo-api reclassify [--no-reset]
//
// It re-runs AI classification over every existing item using the current
// prompt. By default it first wipes all categories so the AI rebuilds a clean,
// specific set (otherwise earlier broad categories keep getting reused). Notes
// (history, dates, text) are never deleted — only category assignments change.
func runReclassifyCLI(args []string) {
	fs := flag.NewFlagSet("reclassify", flag.ExitOnError)
	noReset := fs.Bool("no-reset", false, "keep existing categories instead of wiping and rebuilding the set")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	api := openAPI()
	api.runReclassify(!*noReset)
}

func (api *API) runReclassify(reset bool) {
	apiKey := getEnv("ANTHROPIC_API_KEY", "")
	if apiKey == "" {
		log.Fatal("[reclassify] ANTHROPIC_API_KEY is required")
	}

	if reset {
		log.Println("[reclassify] clearing existing categories (notes are kept)…")
		if err := api.db.Exec("DELETE FROM item_categories").Error; err != nil {
			log.Fatal(err)
		}
		if err := api.db.Exec("DELETE FROM categories").Error; err != nil {
			log.Fatal(err)
		}
		if err := api.db.Model(&Item{}).Where("1 = 1").Update("classified", false).Error; err != nil {
			log.Fatal(err)
		}
	}

	var items []Item
	// Oldest first, so categories accumulate and later notes reuse earlier ones.
	if err := api.db.Order("created_at asc").Find(&items).Error; err != nil {
		log.Fatal(err)
	}

	classified := 0
	for i, it := range items {
		known, _ := api.AllCategoryNames()
		cats, links, err := requestAI(apiKey, it.Text, known)
		if err != nil {
			log.Printf("[reclassify] AI failed for %q: %v", it.Text, err)
			continue
		}
		if e := api.SetAIResults(it.ID, cats, links); e != nil {
			log.Printf("[reclassify] saving AI results failed for %q: %v", it.Text, e)
		}
		if len(cats) > 0 {
			if e := api.ClassifyItem(it.ID, cats); e != nil {
				log.Printf("[reclassify] classify failed for %q: %v", it.Text, e)
			} else {
				classified++
			}
		}
		log.Printf("[reclassify] %d/%d  %-40.40q  cats=%v", i+1, len(items), it.Text, cats)
	}

	log.Printf("[reclassify] done: %d items, %d classified, %d left for the worklist",
		len(items), classified, len(items)-classified)
}
