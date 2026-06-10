// Package feed performs the surgical, fidelity-preserving title rewrite of spec
// §5.1. It loads a whole RSS 2.0 document with beevik/etree, exposes each
// <item>'s (id, title, body), and lets the caller replace only the title text —
// every other byte (enclosures, media, namespaces, CDATA, ...) round-trips
// unchanged. RSS 2.0 only this release; Atom passes through upstream (spec §3).
package feed

import (
	"errors"
	"fmt"
	"strings"

	"github.com/beevik/etree"
)

// Item is the handler-agnostic extracted view of one RSS <item>.
type Item struct {
	ID    string // <guid> → <link> fallback (spec §5.2; no Atom this release)
	Title string // current upstream <title> text
	Body  string // <content:encoded> → <description> fallback, "" if absent
}

// Doc wraps a parsed feed so item titles can be rewritten in place. The titles
// slice is index-aligned with items, so SetTitle(i, ...) targets items()[i].
type Doc struct {
	doc    *etree.Document
	items  []Item
	titles []*etree.Element // per-item <title> element (nil if the item has none)
}

// Parse loads raw RSS 2.0 bytes. CDATA is preserved so untouched subtrees
// round-trip byte-for-byte. A document without an RSS <channel> parses with zero
// items, so a caller can serialize it back unchanged.
func Parse(raw []byte) (*Doc, error) {
	doc := etree.NewDocument()
	doc.ReadSettings = etree.ReadSettings{Permissive: true, PreserveCData: true}
	if err := doc.ReadFromBytes(raw); err != nil {
		return nil, fmt.Errorf("parse feed xml: %w", err)
	}

	d := &Doc{doc: doc}
	root := doc.Root()
	if root == nil {
		return nil, errors.New("feed has no root element")
	}
	channel := root.SelectElement("channel")
	if channel == nil {
		return d, nil // not RSS 2.0 shape; no items to rewrite
	}

	for _, item := range channel.SelectElements("item") {
		titleEl := item.SelectElement("title")
		title := ""
		if titleEl != nil {
			title = titleEl.Text()
		}
		d.items = append(d.items, Item{
			ID:    itemID(item),
			Title: title,
			Body:  itemBody(item),
		})
		d.titles = append(d.titles, titleEl)
	}
	return d, nil
}

// Items returns the extracted items in document order. The index aligns with
// SetTitle.
func (d *Doc) Items() []Item { return d.items }

// SetTitle replaces item i's <title> text only, preserving the original
// CDATA-vs-text wrapping. A no-op when the item has no <title>.
func (d *Doc) SetTitle(i int, title string) {
	el := d.titles[i]
	if el == nil {
		return
	}
	if titleIsCData(el) {
		el.SetCData(title)
	} else {
		el.SetText(title)
	}
}

// Bytes serializes the (possibly modified) document. Untouched nodes are
// reproduced exactly.
func (d *Doc) Bytes() ([]byte, error) {
	return d.doc.WriteToBytes()
}

// itemID applies the spec §5.2 priority: <guid> text, else <link> text, else "".
func itemID(item *etree.Element) string {
	if g := item.SelectElement("guid"); g != nil {
		if t := strings.TrimSpace(g.Text()); t != "" {
			return t
		}
	}
	if l := item.SelectElement("link"); l != nil {
		if t := strings.TrimSpace(l.Text()); t != "" {
			return t
		}
	}
	return ""
}

// itemBody prefers the namespaced <content:encoded>, then <description>.
func itemBody(item *etree.Element) string {
	if c := item.SelectElement("content:encoded"); c != nil {
		return c.Text()
	}
	if d := item.SelectElement("description"); d != nil {
		return d.Text()
	}
	return ""
}

// titleIsCData reports whether the title's character data is a CDATA section, so
// the rewrite can keep the same wrapping.
func titleIsCData(el *etree.Element) bool {
	for _, ch := range el.Child {
		if cd, ok := ch.(*etree.CharData); ok {
			return cd.IsCData()
		}
	}
	return false
}
