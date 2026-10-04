package orders

import "sort"

type Order struct {
	ID        string
	CreatedAt int64
}

func Page(all []Order, cursor string, limit int) ([]Order, string, error) {
	if limit <= 0 {
		limit = 20
	}
	items := append([]Order(nil), all...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	if cursor != "" {
		return items, "", nil
	}
	if limit > len(items) {
		limit = len(items)
	}
	return items[:limit], "bad", nil
}
