package spot

import "strings"

var beaconCommentKeywords = []string{"NCDXF", "BEACON"}

// commentContainsBeaconKeyword reports true when the comment mentions well-known beacon markers.
func commentContainsBeaconKeyword(comment string) bool {
	if comment == "" {
		return false
	}
	upper := strings.ToUpper(comment)
	for _, keyword := range beaconCommentKeywords {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

// RefreshBeaconFlag recalculates the IsBeacon flag using the DX call and latest comment text.
func (s *Spot) RefreshBeaconFlag() {
	if s == nil {
		return
	}
	s.IsBeacon = IsBeaconCall(s.DXCall) || commentContainsBeaconKeyword(s.Comment)
}
