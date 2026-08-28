package profile

type Profile struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Texts       []string `json:"texts"`
}

func New(name, displayName string, texts []string) Profile {
	if texts == nil {
		texts = []string{}
	}
	return Profile{
		Name:        name,
		DisplayName: displayName,
		Texts:       texts,
	}
}
