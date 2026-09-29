package report

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Agents parse -json after one decode already happened, which turns []int
// into []any. Ints must survive that round trip, or levels vanish silently.
func TestIntsSurviveJSONRoundTrip(t *testing.T) {
	item := Item{Path: "a.png", Metrics: map[string]any{"levels": []int{69, 90, 90}}}
	var buf bytes.Buffer
	r := New("img-look", "shown", false)
	r.Add(item)
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Items []Item `json:"items"`
	}
	dec := json.NewDecoder(&buf)
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	// Simulate the generic map an agent actually holds: re-marshal metrics
	// through interface{} so []int becomes []any.
	raw, _ := json.Marshal(decoded.Items[0].Metrics)
	var generic map[string]any
	dec2 := json.NewDecoder(bytes.NewReader(raw))
	dec2.UseNumber()
	if err := dec2.Decode(&generic); err != nil {
		t.Fatal(err)
	}
	back := Item{Metrics: generic}
	lv, ok := back.Ints("levels")
	if !ok || len(lv) != 3 || lv[0] != 69 || lv[1] != 90 || lv[2] != 90 {
		t.Fatalf("levels did not survive: %v", generic["levels"])
	}
}

func TestNumAcceptsJSONNumber(t *testing.T) {
	item := Item{Metrics: map[string]any{"psnr": json.Number("39.5")}}
	if v, ok := item.Num("psnr"); !ok || v < 39 || v > 40 {
		t.Fatalf("Num(json.Number) = %v, %v", v, ok)
	}
}
