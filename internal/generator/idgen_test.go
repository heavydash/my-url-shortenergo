package generator

import (
	"testing"
)

func TestIDGen(t *testing.T) {
	tests := []struct {
		name    string
		wantLen int
	}{
		{"generate id length 8", 8},
		{"generate id length 8 again", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IDGen()
			if err != nil {
				t.Errorf("IDGen() error = %v", err)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("IDGen() = %v, want %v", len(got), tt.wantLen)
			}
			if got == "" {
				t.Errorf("IDGen() returned empty string ")
			}
		})
	}
}

func TestIDGen_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id, err := IDGen()
		if err != nil {
			t.Fatalf("IDGen() error = %v", err)
		}
		if ids[id] {
			t.Errorf("IDGen() returned duplicate, %s", id)
		}
		ids[id] = true
	}
}
