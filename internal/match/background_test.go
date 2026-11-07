package match_test

import (
	"testing"

	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/match"
)

// small is a dense model of params parameters with a window of window
// tokens, at Q8_0 and Q2_K.
func small(id string, tier catalog.Tier, params int64, window int) catalog.Model {
	return catalog.Model{
		ID: id, Name: id, Params: params, ActiveParams: params, ContextLength: window,
		Attention: catalog.Attention{FullLayers: 1, FullKV: 1024},
		Quants: []catalog.Quant{
			{Name: "Q8_0", Bytes: params * 85 / 80, BitsPerWeight: 8.5},
			{Name: "Q2_K", Bytes: params * 3 / 8, BitsPerWeight: 3},
		},
		Tier: tier,
	}
}

func TestBackgroundIsTheFastestSmallModelThatFitsAlongside(t *testing.T) {
	moe := model("moe-c", catalog.TierC, 6, 1024)
	moe.ActiveParams = 1e9
	tests := []struct {
		name   string
		models []catalog.Model
		want   string
	}{
		{"fastest of the small models", []catalog.Model{
			small("small-2b", catalog.TierC, 2e9, 256<<10), small("small-1b", catalog.TierB, 1e9, 256<<10),
		}, "small-1b"},
		{"a window that reaches the main model's comes first", []catalog.Model{
			small("small-2b", catalog.TierC, 2e9, 256<<10), small("short-1b", catalog.TierA, 1e9, 32<<10),
		}, "small-2b"},
		{"not slower than the main model", []catalog.Model{small("slow-6b", catalog.TierB, 6e9, 256<<10)}, ""},
		{"no unreliable models", []catalog.Model{small("tools-d", catalog.TierD, 1e9, 256<<10)}, ""},
		{"no large mixture of experts however fast", []catalog.Model{moe}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := &catalog.Catalog{Models: append([]catalog.Model{model("coder-a", catalog.TierA, 16, 1024)}, tt.models...)}
			r := match.Match(appleResult(20_000, 40_000, 60_000), cat, harnesses())
			best, _ := r.Best()
			if best.Model.ID != "coder-a" {
				t.Fatalf("best = %s, want coder-a", best.Model.ID)
			}
			got := ""
			if r.Background != nil {
				got = r.Background.Model.ID
			}
			if got != tt.want {
				t.Fatalf("background = %q, want %q", got, tt.want)
			}
			if r.Background == nil {
				return
			}
			o := r.Background.Option
			if o.Quant.Name != "Q8_0" || o.MemoryBytes+best.Best.MemoryBytes > int64(r.Budget.Bytes) || o.TokensPerSecond <= best.Best.TokensPerSecond {
				t.Errorf("background option = %+v alongside %d bytes of %d", o, best.Best.MemoryBytes, r.Budget.Bytes)
			}
			if o.MaxContext > best.Best.MaxContext {
				t.Errorf("background window %d is longer than the main model's %d", o.MaxContext, best.Best.MaxContext)
			}
		})
	}
}

func TestBackgroundNeedsRoomBesideTheMainModel(t *testing.T) {
	cat := &catalog.Catalog{Models: []catalog.Model{
		model("coder-a", catalog.TierA, 22, 1024),
		small("small-2b", catalog.TierC, 2e9, 256<<10),
	}}
	r := match.Match(appleResult(20_000, 40_000), cat, harnesses())
	if best, _ := r.Best(); best.Model.ID != "coder-a" || r.Background != nil {
		t.Errorf("best %s, background %+v; want coder-a alone, with no room left beside it", best.Model.ID, r.Background)
	}
}

func TestBackgroundForFollowsTheChosenModel(t *testing.T) {
	cat := &catalog.Catalog{Models: []catalog.Model{
		model("coder-a", catalog.TierA, 16, 1024),
		model("coder-b", catalog.TierB, 22, 1024),
		small("small-2b", catalog.TierC, 2e9, 256<<10),
	}}
	r := match.Match(appleResult(20_000, 40_000), cat, harnesses())
	if r.Background == nil || r.Background.Model.ID != "small-2b" {
		t.Fatalf("background = %+v, want small-2b beside coder-a", r.Background)
	}
	for _, c := range r.Candidates {
		if c.Model.ID != "coder-b" {
			continue
		}
		if bg, ok := r.BackgroundFor(c); ok {
			t.Errorf("background beside coder-b = %s, want none: coder-b leaves no room", bg.Model.ID)
		}
		return
	}
	t.Fatal("coder-b is not a candidate")
}
