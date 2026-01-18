package telnet

import (
	"bytes"
	"testing"
	"time"

	"dxcluster/cty"
	"dxcluster/pathreliability"
	"dxcluster/spot"

	"howett.net/plist"
)

func TestPathGlyphsInferGrid2FromCTY(t *testing.T) {
	now := time.Now().UTC()
	cfg := pathreliability.DefaultConfig()
	predictor := pathreliability.NewPredictor(cfg, []string{"20m"})

	userGrid := "JN75"
	userCell := pathreliability.EncodeCell(userGrid)
	userGrid2 := pathreliability.EncodeGrid2(userGrid)
	dxGrid2 := "JN"

	predictor.Update(pathreliability.BucketBaseline, userCell, pathreliability.InvalidCell, userGrid2, dxGrid2, "20m", -10, 1.0, now, false)

	db := loadCTYForTest(t, map[string]cty.PrefixInfo{
		"S50": {Latitude: 46.0, Longitude: 14.0},
	})

	srv := &Server{
		pathPredictor:  predictor,
		pathDisplay:    true,
		pathInferGrid2: true,
		ctyLookup: func() *cty.CTYDatabase {
			return db
		},
	}
	client := &Client{
		grid:     userGrid,
		gridCell: pathreliability.InvalidCell,
	}
	sp := spot.NewSpot("S50DXS", "S53ZO", 14074.0, "FT8")
	sp.DXMetadata.Grid = ""

	glyph := srv.pathGlyphsForClient(client, sp)
	if glyph != "+" {
		t.Fatalf("expected inferred glyph '+', got %q", glyph)
	}

	srv.pathInferGrid2 = false
	glyph = srv.pathGlyphsForClient(client, sp)
	if glyph != "" {
		t.Fatalf("expected no glyph without CTY inference, got %q", glyph)
	}
}

func loadCTYForTest(t *testing.T, entries map[string]cty.PrefixInfo) *cty.CTYDatabase {
	t.Helper()
	var buf bytes.Buffer
	encoder := plist.NewEncoder(&buf)
	if err := encoder.Encode(entries); err != nil {
		t.Fatalf("encode plist: %v", err)
	}
	db, err := cty.LoadCTYDatabaseFromReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("load cty: %v", err)
	}
	return db
}
