package build

import "testing"

func TestTiersTableShape(t *testing.T) {
	t.Parallel()

	wantNames := []string{"nano", "small", "medium", "large"}
	for _, name := range wantNames {
		tier, ok := Tiers[name]
		if !ok {
			t.Fatalf("Tiers missing %q", name)
		}
		if tier.Name != name {
			t.Errorf("Tiers[%q].Name = %q, want %q", name, tier.Name, name)
		}
		if tier.MemoryMB <= 0 || tier.BuildMemoryMB <= 0 {
			t.Errorf("Tiers[%q] has non-positive memory: runtime=%d build=%d",
				name, tier.MemoryMB, tier.BuildMemoryMB)
		}
		if tier.CPUCores <= 0 || tier.BuildCPUCores <= 0 {
			t.Errorf("Tiers[%q] has non-positive CPU: runtime=%v build=%v",
				name, tier.CPUCores, tier.BuildCPUCores)
		}
		if tier.BuildMemoryMB <= tier.MemoryMB {
			t.Errorf("Tiers[%q]: build memory (%d) must exceed runtime memory (%d) — builds peak higher than steady-state",
				name, tier.BuildMemoryMB, tier.MemoryMB)
		}
	}

	// Small must equal the legacy hardcoded values so the migration is byte-identical.
	small := Tiers["small"]
	if small.MemoryMB != 512 || small.CPUCores != 1.0 {
		t.Errorf("Small tier must equal legacy 512 MB / 1.0 CPU, got %d MB / %.2f CPU",
			small.MemoryMB, small.CPUCores)
	}

	// Build cap must fit inside the platform-wide BuildKit ceiling (6 GB).
	for name, tier := range Tiers {
		if tier.BuildMemoryMB > 6144 {
			t.Errorf("Tiers[%q] build memory %d MB exceeds BuildKit container cap of 6 GB",
				name, tier.BuildMemoryMB)
		}
	}
}

func TestTierOrDefault(t *testing.T) {
	t.Parallel()

	if got := TierOrDefault("large"); got.Name != "large" {
		t.Errorf("TierOrDefault(large).Name = %q, want large", got.Name)
	}
	if got := TierOrDefault(""); got.Name != "small" {
		t.Errorf("TierOrDefault(empty).Name = %q, want small (default)", got.Name)
	}
	if got := TierOrDefault("does-not-exist"); got.Name != "small" {
		t.Errorf("TierOrDefault(invalid).Name = %q, want small (default)", got.Name)
	}
}

func TestIsValidTier(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"nano":           true,
		"small":          true,
		"medium":         true,
		"large":          true,
		"":               false,
		"NANO":           false, // case-sensitive — handler lowercases first
		"xl":             false,
		"small-and-some": false,
	}
	for in, want := range cases {
		if got := IsValidTier(in); got != want {
			t.Errorf("IsValidTier(%q) = %v, want %v", in, got, want)
		}
	}
}
