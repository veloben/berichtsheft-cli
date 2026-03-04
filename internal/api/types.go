package api

type YearResponse struct {
	Number string        `json:"Number"`
	Weeks  []WeekSummary `json:"Weeks"`
}

type WeekSummary struct {
	Number string           `json:"Number"`
	Days   []DaySummaryItem `json:"Days"`
}

type DaySummaryItem struct {
	Number      string `json:"Number"`
	HasApproval bool   `json:"HasApproval"`
}

type DayData struct {
	MdData   string   `json:"MdData"`
	Metadata Metadata `json:"Metadata"`
}

type Metadata struct {
	Approved       bool            `json:"Approved"`
	TimeSpent      int             `json:"TimeSpent"`
	Qualifications []Qualification `json:"Qualifications"`
	Comments       []string        `json:"Comments"`
	Location       string          `json:"Location"`
	Status         string          `json:"Status"`
}

type Qualification struct {
	Name        string `json:"Name"`
	Description string `json:"Description"`
}
