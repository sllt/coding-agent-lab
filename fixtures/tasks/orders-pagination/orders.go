package orders

import (
	"fmt"
	"sort"
	"strconv"
)

type Order struct {
	ID        string
	CreatedAt int64
}

// Page is the buggy baseline. The cursor is only a timestamp, so rows that
// share created_at are repeated on the next page.
func Page(all []Order, cursor string, limit int) ([]Order, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	items := append([]Order(nil), all...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	start := 0
	if cursor != "" {
		ts, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		for start < len(items) && items[start].CreatedAt > ts {
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
		next = strconv.FormatInt(page[len(page)-1].CreatedAt, 10)
	}
	return page, next, nil
}
