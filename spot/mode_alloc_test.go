package spot

import "testing"

// Purpose: Ensure 40m allocation treats lower band as CW/data, not LSB voice.
// Context: Regression for inference returning LSB at 7011 kHz.
func TestFinalizeModePrefersCWBelow40mVoiceSegment(t *testing.T) {
	mode := FinalizeMode("", 7011.0)
	if mode != "CW" {
		t.Fatalf("expected CW for 7011.0 kHz, got %q", mode)
	}
}
