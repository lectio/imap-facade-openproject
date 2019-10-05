package hal

//
// Project
//

type Project struct {
	ResourceObject
}

func NewProject() *Project {
	return &Project{
		ResourceObject{
			Type: "Project",
		},
	}
}

func (res *Project) Name() string {
	return res.getString("name")
}

func (res *Project) Description() string {
	return res.getString("description")
}

// Register Factories
func init() {
	resourceTypes["Project"] = func() Resource {
		return NewProject()
	}
}
