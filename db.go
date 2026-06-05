package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CreateItem records a new printed item (the permanent history row that also
// serves as the printer queue until PrintedAt is set).
func (api *API) CreateItem(text string) (*Item, error) {
	item := &Item{Text: text, CreatedAt: time.Now()}
	if err := api.db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

// PendingTexts returns the text of every item not yet printed, oldest first.
// This is what the printer worker fetches from GET /messages.
func (api *API) PendingTexts() ([]string, error) {
	var items []Item
	err := api.db.Where("printed_at IS NULL").Order("id asc").Find(&items).Error
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.Text
	}
	return texts, nil
}

// MarkPending Printed stamps every not-yet-printed item as printed now. This is
// what DELETE /messages does: it clears the worker's queue without destroying
// the history.
func (api *API) MarkPendingPrinted() error {
	now := time.Now()
	return api.db.Model(&Item{}).Where("printed_at IS NULL").Update("printed_at", now).Error
}

// HistoryItems returns items for the admin site, newest first, with categories
// preloaded. If category is non-empty, only items in that category are returned.
func (api *API) HistoryItems(category string) ([]Item, error) {
	var items []Item
	q := api.db.Preload("Categories").Order("created_at desc")
	if category != "" {
		q = q.Joins("JOIN item_categories ic ON ic.item_id = items.id").
			Joins("JOIN categories c ON c.id = ic.category_id").
			Where("c.name = ?", category)
	}
	if err := q.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// UnclassifiedItems returns items the user has not yet categorized, newest
// first, with their AI suggestions preloaded.
func (api *API) UnclassifiedItems() ([]Item, error) {
	var items []Item
	err := api.db.Preload("Categories").
		Where("classified = ?", false).
		Order("created_at desc").
		Find(&items).Error
	return items, err
}

// AllCategoryNames returns the distinct, sorted set of category names in use.
func (api *API) AllCategoryNames() ([]string, error) {
	var cats []Category
	if err := api.db.Order("name asc").Find(&cats).Error; err != nil {
		return nil, err
	}
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	return names, nil
}

// canonicalCategoryKey normalizes a category name so simple case and
// singular/plural variants collapse to one key ("Cocktail"/"cocktails" ->
// "cocktail"), used to avoid creating duplicate categories.
func canonicalCategoryKey(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasSuffix(s, "ies") && len(s) > 3:
		return s[:len(s)-3] + "y" // berries -> berry
	case strings.HasSuffix(s, "ss"):
		return s // glass, dress — don't strip
	case strings.HasSuffix(s, "s") && len(s) > 1:
		return s[:len(s)-1] // cocktails -> cocktail
	}
	return s
}

// resolveCategoryTx returns the category matching name — exactly, or by
// canonical singular/plural key — creating a new one only if none exists.
func resolveCategoryTx(tx *gorm.DB, name string) (Category, error) {
	var existing Category
	if err := tx.Where("name = ?", name).First(&existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Category{}, err
	}
	key := canonicalCategoryKey(name)
	var all []Category
	if err := tx.Find(&all).Error; err != nil {
		return Category{}, err
	}
	for _, c := range all {
		if canonicalCategoryKey(c.Name) == key {
			return c, nil
		}
	}
	nc := Category{Name: name}
	if err := tx.Create(&nc).Error; err != nil {
		return Category{}, err
	}
	return nc, nil
}

// UpdateItemText changes a note's text.
func (api *API) UpdateItemText(itemID uint, text string) error {
	return api.db.Model(&Item{}).Where("id = ?", itemID).Update("text", text).Error
}

// ClassifyItem replaces an item's categories with the given names (creating any
// new categories) and marks it classified. Empty/blank names are ignored.
func (api *API) ClassifyItem(itemID uint, names []string) error {
	return api.db.Transaction(func(tx *gorm.DB) error {
		var item Item
		if err := tx.First(&item, itemID).Error; err != nil {
			return err
		}

		cats := make([]Category, 0, len(names))
		seen := map[string]bool{}
		for _, raw := range names {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			// Dedup within this request by canonical (case/plural-insensitive) key.
			key := canonicalCategoryKey(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			cat, err := resolveCategoryTx(tx, name)
			if err != nil {
				return err
			}
			cats = append(cats, cat)
		}

		// Replace the item's category associations with exactly this set.
		if err := tx.Model(&item).Association("Categories").Replace(cats); err != nil {
			return err
		}
		return tx.Model(&item).Update("classified", true).Error
	})
}

// SetAIResults stores the AI's proposed categories and extracted links for an
// item, both as JSON. Categories are kept separate from confirmed Categories so
// the user still confirms before anything is filed; links are informational and
// shown immediately.
func (api *API) SetAIResults(itemID uint, suggestions []string, links []Link) error {
	sData, err := json.Marshal(suggestions)
	if err != nil {
		return err
	}
	lData, err := json.Marshal(links)
	if err != nil {
		return err
	}
	return api.db.Model(&Item{}).Where("id = ?", itemID).
		Updates(map[string]interface{}{"suggested": string(sData), "links": string(lData)}).Error
}

// suggestionList decodes an item's stored AI suggestions.
func (it *Item) suggestionList() []string {
	if strings.TrimSpace(it.Suggested) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(it.Suggested), &out); err != nil {
		return nil
	}
	return out
}

// linkList decodes an item's stored AI-extracted links.
func (it *Item) linkList() []Link {
	if strings.TrimSpace(it.Links) == "" {
		return nil
	}
	var out []Link
	if err := json.Unmarshal([]byte(it.Links), &out); err != nil {
		return nil
	}
	return out
}

// categoryNames returns this item's confirmed category names, sorted.
func (it *Item) categoryNames() []string {
	names := make([]string, len(it.Categories))
	for i, c := range it.Categories {
		names[i] = c.Name
	}
	sort.Strings(names)
	return names
}
