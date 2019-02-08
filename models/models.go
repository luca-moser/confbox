package models

type ConfRate struct {
	Avg5  float64 `json:"avg_5"`
	Avg10 float64 `json:"avg_10"`
	Avg15 float64 `json:"avg_15"`
	Avg30 float64 `json:"avg_30"`
}

type Response struct {
	Results ConfRate      `json:"results"`
	Config  ExposedConfig `json:"config"`
}

type ExposedConfig struct {
	MWM             uint64 `json:"mwm"`
	GTTADepth       uint64 `json:"gtta_depth"`
	TransferPolling struct {
		Interval uint64 `json:"interval"`
	} `json:"transfer_polling"`
	PromoteReattach struct {
		Enabled  bool   `json:"enabled"`
		Interval uint64 `json:"interval"`
	} `json:"promote_reattach"`
}
