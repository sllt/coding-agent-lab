package filter

type Order struct {
	ID     string
	Status string
}

func List(all []Order) []Order { return append([]Order(nil), all...) }

func FilterByStatus(all []Order, status string) []Order {
	return append([]Order(nil), all...)
}
