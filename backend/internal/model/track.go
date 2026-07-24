package model

type Track struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Filename string  `json:"filename"`
	URL      string  `json:"url"`
	Duration float64 `json:"duration"`
	Category string  `json:"category,omitempty"`
	Theme    string  `json:"theme,omitempty"`
}

