package hal

//
// Collection
//

type Collection struct {
	ResourceObject
}

func NewCollection() *Collection {
	return &Collection{
		ResourceObject{
			Type: "Collection",
		},
	}
}

func (res *Project) Total() int {
	return res.getInt("total")
}

func (res *Project) Count() int {
	return res.getInt("count")
}

func (res *Collection) Items() []Resource {
	items, ok := res.Embedded["elements"]
	if ok {
		return items
	}
	return nil
}

// Register Resource Factories
func init() {
	resourceTypes["Collection"] = func() Resource {
		return NewCollection()
	}
}
