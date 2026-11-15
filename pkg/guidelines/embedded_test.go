package guidelines

import "testing"

func TestLoadEmbeddedGuidelines(t *testing.T) {
	cats, err := loadEmbeddedGuidelines()
	if err != nil {
		t.Fatalf("loadEmbeddedGuidelines: %v", err)
	}
	if len(cats) < 12 {
		t.Fatalf("want >= 12 embedded categories, got %d", len(cats))
	}
	rules := 0
	for _, c := range cats {
		rules += len(c.Guidelines)
	}
	if rules < 86 {
		t.Fatalf("want >= 86 embedded rules, got %d", rules)
	}
}
