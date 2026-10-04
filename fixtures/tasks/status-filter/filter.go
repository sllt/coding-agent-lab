package filter

type Order struct {
	ID     string
	Status string
}

func List(all []Order) []Order {
	return append([]Order(nil), all...)
}

// FilterByStatus is intentionally unimplemented in the baseline.
func FilterByStatus(all []Order, status string) []Order {
	panic("status filter is not implemented")
}
