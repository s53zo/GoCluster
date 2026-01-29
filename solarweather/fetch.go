package solarweather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

type conditionalFetcher struct {
	url          string
	etag         string
	lastModified string
	client       *http.Client
}

func (f *conditionalFetcher) Fetch(ctx context.Context) ([]byte, bool, error) {
	if f == nil {
		return nil, false, fmt.Errorf("nil fetcher")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, false, err
	}
	if f.etag != "" {
		req.Header.Set("If-None-Match", f.etag)
	}
	if f.lastModified != "" {
		req.Header.Set("If-Modified-Since", f.lastModified)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		f.etag = etag
	}
	if last := resp.Header.Get("Last-Modified"); last != "" {
		f.lastModified = last
	}
	return body, true, nil
}

type goesEntry struct {
	TimeTag string  `json:"time_tag"`
	Flux    float64 `json:"flux"`
	Energy  string  `json:"energy"`
}

type goesSample struct {
	Time time.Time
	Flux float64
}

func parseGOES(body []byte, energyBand string) ([]goesSample, bool) {
	var entries []goesEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, false
	}
	samples := make([]goesSample, 0, len(entries))
	for _, entry := range entries {
		if entry.Energy != energyBand {
			continue
		}
		t, err := time.Parse(time.RFC3339, entry.TimeTag)
		if err != nil {
			continue
		}
		samples = append(samples, goesSample{
			Time: t.UTC(),
			Flux: entry.Flux,
		})
	}
	if len(samples) == 0 {
		return nil, false
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Time.Before(samples[j].Time)
	})
	return samples, true
}

func parseKp(body []byte) (float64, time.Time, bool) {
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, time.Time{}, false
	}
	if len(rows) <= 1 {
		return 0, time.Time{}, false
	}
	layout := "2006-01-02 15:04:05.000"
	var latest time.Time
	var kp float64
	found := false
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 2 {
			continue
		}
		t, err := time.Parse(layout, row[0])
		if err != nil {
			continue
		}
		val, err := parseFloat(row[1])
		if err != nil {
			continue
		}
		t = t.UTC()
		if !found || t.After(latest) {
			latest = t
			kp = val
			found = true
		}
	}
	return kp, latest, found
}

func parseFloat(s string) (float64, error) {
	var v float64
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v, nil
	}
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}
