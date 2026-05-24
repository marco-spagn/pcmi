package embedding

import "testing"

func TestValidateModelDimension(t *testing.T) {
	// Known compatible model — must pass.
	if err := ValidateModelDimension("text-embedding-3-small"); err != nil {
		t.Errorf("text-embedding-3-small should be valid: %v", err)
	}
	if err := ValidateModelDimension("text-embedding-ada-002"); err != nil {
		t.Errorf("ada-002 should be valid: %v", err)
	}

	// Known incompatible model — must fail with clear message.
	err := ValidateModelDimension("text-embedding-3-large")
	if err == nil {
		t.Fatal("text-embedding-3-large (3072d) should fail against DB dim 1536")
	}
	t.Logf("correct error: %v", err)

	// Unknown model — unknown is allowed (custom/local models).
	if err := ValidateModelDimension("my-custom-model-v2"); err != nil {
		t.Errorf("unknown model should be allowed through: %v", err)
	}

	// Empty string — treated as unknown, allowed.
	if err := ValidateModelDimension(""); err != nil {
		t.Errorf("empty model should be allowed: %v", err)
	}
}

func TestDBVectorDimension_Constant(t *testing.T) {
	if DBVectorDimension != 1536 {
		t.Errorf("DBVectorDimension = %d, want 1536 (must match migration 001_init.sql)", DBVectorDimension)
	}
}
