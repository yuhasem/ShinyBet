package state

// The JSON parsed state received from the bot
type State struct {
	Encounter struct {
		IsShiny     bool `json:"is_shiny"`
		IsAntiShiny bool `json:"is_anti_shiny"`
	} `json:"encounter"`
	Stats struct {
		CurrentPhase struct {
			StartTime  string `json:"start_time"`
			Encounters int    `json:"encounters"`
		} `json:"current_phase"`
	} `json:"stats"`
}
