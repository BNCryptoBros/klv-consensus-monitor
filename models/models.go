package models

type MonitoredValidator struct {
	BLSKey      string `yaml:"blsKey"`
	DisplayName string `yaml:"displayName"`
}

type ValidatorInfo struct {
	BLSPublicKey	string `json:"-"`
	ValidatorStatus string `json:"ValidatorStatus"`
}

type BlockInfo struct {
	Epoch  int    `json:"epoch"`
}

type ValidatorStatisticsResponse struct {
	Data struct {
		Statistics map[string]ValidatorInfo `json:"statistics"`
	} `json:"data"`
}

type BlockListResponse struct {
	Data struct {
		Blocks []BlockInfo `json:"blocks"`
	} `json:"data"`
}

type ValidatorState struct {
	BLSKey      string
	DisplayName string
	Status      string
	Epoch       int
}
