package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) GetYear(year int) (*YearResponse, error) {
	url := fmt.Sprintf("%s/api/year/%d", c.baseURL, year)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get year failed: HTTP %d", resp.StatusCode)
	}

	var data YearResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *Client) GetDay(year, week, day int) (*DayData, int, error) {
	url := fmt.Sprintf("%s/api/%d/%d/%d", c.baseURL, year, week, day)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, http.StatusNotFound, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("get day failed: HTTP %d", resp.StatusCode)
	}

	var data DayData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, resp.StatusCode, err
	}

	NormalizeDayData(&data)
	return &data, resp.StatusCode, nil
}

func (c *Client) SaveDay(year, week, day int, data DayData) error {
	NormalizeDayData(&data)

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/%d/%d/%d", c.baseURL, year, week, day)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("save day failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

func NormalizeDayData(d *DayData) {
	if d == nil {
		return
	}
	d.Metadata.Location = NormalizeLocation(d.Metadata.Location)
	d.Metadata.Status = NormalizeStatus(d.Metadata.Status)
	d.Metadata.TimeSpent = ClampTime(d.Metadata.TimeSpent)
	if d.Metadata.Qualifications == nil {
		d.Metadata.Qualifications = []Qualification{}
	}
	if d.Metadata.Comments == nil {
		d.Metadata.Comments = []string{}
	}
}

func NormalizeLocation(v string) string {
	if v == "school" || v == "schule" {
		return "schule"
	}
	return "betrieb"
}

func NormalizeStatus(v string) string {
	switch v {
	case "school":
		return "schulzeit"
	case "holiday":
		return "urlaub"
	case "other":
		return "sonstiges"
	case "anwesend", "schulzeit", "urlaub", "sonstiges":
		return v
	default:
		return "anwesend"
	}
}

func ClampTime(v int) int {
	if v < 0 {
		return 0
	}
	if v > 12 {
		return 12
	}
	return v
}
