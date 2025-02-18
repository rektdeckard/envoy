package envoy

type Dimensioned struct {
	Units string `json:"units"`
	Value string `json:"value"`
}

type Size struct {
	Length int    `json:"length"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Units  string `json:"units"`
}

type Value struct {
	Currency string  `json:"currency"`
	Value    float64 `json:"value"`
}

type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
