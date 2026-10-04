package orders

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Order struct {
	ID        string
	CreatedAt int64
}

func Page(all []Order, cursor string, limit int) ([]Order, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	items := append([]Order(nil), all...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	start := 0
	if cursor != "" {
		parts := strings.Split(cursor, "|")
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		ts, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || parts[1] == "" {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		id := parts[1]
		for start < len(items) {
			item := items[start]
			if item.CreatedAt < ts || (item.CreatedAt == ts && item.ID < id) {
				break
			}
			if item.CreatedAt == ts && item.ID == id {
				start++
				break
			}
			start++
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	next := ""
	if end < len(items) && len(page) > 0 {
		last := page[len(page)-1]
		next = strconv.FormatInt(last.CreatedAt, 10) + "|" + last.ID
	}
	return page, next, nil
}
