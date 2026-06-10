// file: internal/dedup/unified/config_test.go
// version: 1.0.0
// guid: 4f7a2c9b-8e3d-4b6f-a1c5-9e7b3d2f6a8c

package unified

import (
	"testing"

	"github.com/spf13/viper"
)

// resetViper clears all Viper state so tests don't bleed into each other.
func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
}

// TestLoadScoreConfig_DefaultsWithNoOverrides verifies that LoadScoreConfig
// returns the SPEC defaults when no Viper keys are set.
func TestLoadScoreConfig_DefaultsWithNoOverrides(t *testing.T) {
	resetViper(t)

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("LoadScoreConfig with no overrides failed: %v", err)
	}

	if cfg.BandCertainMin != 97.0 {
		t.Errorf("default BandCertainMin: expected 97.0, got %.2f", cfg.BandCertainMin)
	}
	if cfg.BandHighMin != 90.0 {
		t.Errorf("default BandHighMin: expected 90.0, got %.2f", cfg.BandHighMin)
	}
	if cfg.BandMediumMin != 75.0 {
		t.Errorf("default BandMediumMin: expected 75.0, got %.2f", cfg.BandMediumMin)
	}
	if cfg.BandReviewMin != 60.0 {
		t.Errorf("default BandReviewMin: expected 60.0, got %.2f", cfg.BandReviewMin)
	}

	// Spot-check a few per-kind defaults.
	exactFile, ok := cfg.Signals[string(SigExactFile)]
	if !ok {
		t.Fatal("expected SigExactFile in default config")
	}
	if exactFile.Base != 1.0 {
		t.Errorf("SigExactFile base: expected 1.0, got %.4f", exactFile.Base)
	}
}

// TestLoadScoreConfig_BandOverrides verifies that band thresholds can be
// overridden individually via Viper.
func TestLoadScoreConfig_BandOverrides(t *testing.T) {
	resetViper(t)

	viper.Set("dedup.signals.band_certain_min", 98.0)
	viper.Set("dedup.signals.band_high_min", 92.0)
	viper.Set("dedup.signals.band_medium_min", 80.0)
	viper.Set("dedup.signals.band_review_min", 65.0)

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("LoadScoreConfig with band overrides failed: %v", err)
	}

	if cfg.BandCertainMin != 98.0 {
		t.Errorf("BandCertainMin: expected 98.0, got %.2f", cfg.BandCertainMin)
	}
	if cfg.BandHighMin != 92.0 {
		t.Errorf("BandHighMin: expected 92.0, got %.2f", cfg.BandHighMin)
	}
	if cfg.BandMediumMin != 80.0 {
		t.Errorf("BandMediumMin: expected 80.0, got %.2f", cfg.BandMediumMin)
	}
	if cfg.BandReviewMin != 65.0 {
		t.Errorf("BandReviewMin: expected 65.0, got %.2f", cfg.BandReviewMin)
	}
}

// TestLoadScoreConfig_PerKindBoostOverride verifies that per-kind boost
// values can be overridden via Viper.
func TestLoadScoreConfig_PerKindBoostOverride(t *testing.T) {
	resetViper(t)

	// Override the duration boost to 8.0.
	viper.Set("dedup.signals."+string(SigDuration), true) // marks it as "set"
	viper.Set("dedup.signals."+string(SigDuration)+".boost", 8.0)

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("LoadScoreConfig with kind override failed: %v", err)
	}

	if cfg.BoostFor(SigDuration) != 8.0 {
		t.Errorf("duration boost after override: expected 8.0, got %.2f", cfg.BoostFor(SigDuration))
	}
	// folder_path should be unchanged.
	if cfg.BoostFor(SigFolderPath) != 3.0 {
		t.Errorf("folder_path boost should be unchanged: expected 3.0, got %.2f", cfg.BoostFor(SigFolderPath))
	}
}

// TestLoadScoreConfig_PerKindBaseOverride verifies that per-kind base
// values can be overridden via Viper.
func TestLoadScoreConfig_PerKindBaseOverride(t *testing.T) {
	resetViper(t)

	viper.Set("dedup.signals."+string(SigMetaFuzzy), true)
	viper.Set("dedup.signals."+string(SigMetaFuzzy)+".base", 0.75)
	viper.Set("dedup.signals."+string(SigMetaFuzzy)+".min_confidence", 0.75)
	viper.Set("dedup.signals."+string(SigMetaFuzzy)+".max_confidence", 0.90)

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("LoadScoreConfig with base override failed: %v", err)
	}

	kc := cfg.Signals[string(SigMetaFuzzy)]
	if kc.Base != 0.75 {
		t.Errorf("SigMetaFuzzy base: expected 0.75, got %.4f", kc.Base)
	}
	if kc.MinConfidence != 0.75 {
		t.Errorf("SigMetaFuzzy min_confidence: expected 0.75, got %.4f", kc.MinConfidence)
	}
	if kc.MaxConfidence != 0.90 {
		t.Errorf("SigMetaFuzzy max_confidence: expected 0.90, got %.4f", kc.MaxConfidence)
	}
}

// TestLoadScoreConfig_ScaleOverride verifies scale override.
func TestLoadScoreConfig_ScaleOverride(t *testing.T) {
	resetViper(t)

	viper.Set("dedup.signals."+string(SigEmbedHigh), true)
	viper.Set("dedup.signals."+string(SigEmbedHigh)+".scale", 2.0)

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	kc := cfg.Signals[string(SigEmbedHigh)]
	if kc.Scale != 2.0 {
		t.Errorf("SigEmbedHigh scale: expected 2.0, got %.4f", kc.Scale)
	}
}

// TestLoadScoreConfig_InvalidBandsReturnError verifies that LoadScoreConfig
// returns an error when band overrides produce an invalid ordering.
func TestLoadScoreConfig_InvalidBandsReturnError(t *testing.T) {
	resetViper(t)

	// Set CERTAIN lower than HIGH — invalid ordering.
	viper.Set("dedup.signals.band_certain_min", 80.0)
	viper.Set("dedup.signals.band_high_min", 90.0)

	_, err := LoadScoreConfig()
	if err == nil {
		t.Error("expected error for invalid band ordering, got nil")
	}
}

// TestLoadScoreConfig_InvalidConfidenceReturnError verifies that a
// per-kind confidence override outside (0, 1] returns an error.
func TestLoadScoreConfig_InvalidConfidenceReturnError(t *testing.T) {
	resetViper(t)

	// Set SigExactFile max_confidence to 1.5 — invalid.
	viper.Set("dedup.signals."+string(SigExactFile), true)
	viper.Set("dedup.signals."+string(SigExactFile)+".min_confidence", 0.5)
	viper.Set("dedup.signals."+string(SigExactFile)+".max_confidence", 1.5)

	_, err := LoadScoreConfig()
	if err == nil {
		t.Error("expected error for confidence > 1, got nil")
	}
}

// TestLoadScoreConfig_AllKindsCanBeOverridden exercises the loop over all
// signal kinds in LoadScoreConfig, touching the non-set path for each.
func TestLoadScoreConfig_AllKindsCanBeOverridden(t *testing.T) {
	resetViper(t)

	// Set a known-valid override for every kind via the kind-level key.
	// This exercises each branch of the per-kind loop in LoadScoreConfig.
	allKinds := []SignalKind{
		SigExactFile, SigExactAcoustID, SigISBNASIN, SigLSHAcoustID,
		SigEmbedHigh, SigMetaSrcHash, SigMetaFuzzy, SigEmbedMedium,
		SigDuration, SigFolderPath,
	}
	for _, k := range allKinds {
		key := "dedup.signals." + string(k)
		viper.Set(key, true) // marks the key as set so the branch executes
	}

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("LoadScoreConfig with all kinds set: %v", err)
	}

	// Just verify all kinds are present with valid defaults.
	for _, k := range allKinds {
		if _, ok := cfg.Signals[string(k)]; !ok {
			t.Errorf("missing kind %q after override", k)
		}
	}
}

// TestScoreConfig_BoostForMissingKind verifies that BoostFor returns 0
// when the kind is not present in the config map.
func TestScoreConfig_BoostForMissingKind(t *testing.T) {
	cfg := DefaultScoreConfig()
	// Delete a kind from the map.
	delete(cfg.Signals, string(SigDuration))

	if b := cfg.BoostFor(SigDuration); b != 0.0 {
		t.Errorf("BoostFor missing kind: expected 0, got %.2f", b)
	}
}

// TestValidate_NegativeReviewMin verifies that a negative BandReviewMin
// returns a validation error.
func TestValidate_NegativeReviewMin(t *testing.T) {
	cfg := DefaultScoreConfig()
	cfg.BandReviewMin = -1.0

	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative BandReviewMin, got nil")
	}
}

// TestValidate_MediumHighOrderingError verifies band_high_min > band_medium_min.
func TestValidate_MediumHighOrderingError(t *testing.T) {
	cfg := DefaultScoreConfig()
	cfg.BandHighMin = 70.0  // lower than medium
	cfg.BandMediumMin = 80.0
	// Now band_high < band_medium — invalid.

	if err := cfg.Validate(); err == nil {
		t.Error("expected error for high < medium band ordering, got nil")
	}
}

// TestValidate_MediumReviewOrderingError verifies band_medium_min > band_review_min.
func TestValidate_MediumReviewOrderingError(t *testing.T) {
	cfg := DefaultScoreConfig()
	cfg.BandMediumMin = 55.0  // lower than review
	cfg.BandReviewMin = 60.0

	if err := cfg.Validate(); err == nil {
		t.Error("expected error for medium < review band ordering, got nil")
	}
}

// TestLoadScoreConfig_PerKindMinConfidenceOverride exercises the min_confidence branch.
func TestLoadScoreConfig_PerKindMinConfidenceOverride(t *testing.T) {
	resetViper(t)

	viper.Set("dedup.signals."+string(SigEmbedMedium), true)
	viper.Set("dedup.signals."+string(SigEmbedMedium)+".min_confidence", 0.70)

	cfg, err := LoadScoreConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	kc := cfg.Signals[string(SigEmbedMedium)]
	if kc.MinConfidence != 0.70 {
		t.Errorf("min_confidence override: expected 0.70, got %.4f", kc.MinConfidence)
	}
}
