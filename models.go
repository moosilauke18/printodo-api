package main

import (
	"time"

	"gorm.io/gorm"
)

type API struct {
	Username   string
	Password   string
	Env        string
	SigningKey []byte
	db         *gorm.DB
}

// Message is the JSON body posted by the mobile app to /message.
type Message struct {
	Message string
}

type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Item is a single thing that was sent to the printer. It doubles as the
// printer queue (PrintedAt == nil means "not yet printed") and the permanent,
// browsable history. Categories are user-confirmed; Suggested holds the AI's
// proposals (a JSON array of strings) awaiting confirmation.
type Item struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Text       string     `gorm:"type:text" json:"text"`
	CreatedAt  time.Time  `json:"created_at"`
	PrintedAt  *time.Time `json:"printed_at"`
	Classified bool       `gorm:"not null;default:false" json:"classified"`
	Suggested  string     `gorm:"type:text" json:"-"`
	Links      string     `gorm:"type:text" json:"-"`
	Categories []Category `gorm:"many2many:item_categories;" json:"categories"`
}

// Link is a brand/product the AI found in a note, paired with a URL to its
// official site (or a web search for it when no confident official URL exists).
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Category is a label an item can belong to (e.g. "hunting", "bow", "scotch").
// Items and categories are many-to-many: one item can live under several.
type Category struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"uniqueIndex;not null" json:"name"`
}
