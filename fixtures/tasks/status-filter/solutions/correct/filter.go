package filter

type Order struct {
	ID     string
	Status string
}

func List(all []Order) []Order {
	return append([]Order(nil), all...)
}

func FilterByStatus(all []Order, status string) []Order {
	var out []Order
	for _, item := range all {
		if item.Status == status {
			out = append(out, item)
		}
	}
	return out
}
