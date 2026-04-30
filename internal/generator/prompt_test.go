package generator

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestPromptBool(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
	}{
		{name: "empty input uses default true", input: "\n", defaultYes: true, want: true},
		{name: "empty input uses default false", input: "\n", defaultYes: false, want: false},
		{name: "y selects yes", input: "y\n", defaultYes: false, want: true},
		{name: "yes selects yes", input: "yes\n", defaultYes: false, want: true},
		{name: "Y selects yes", input: "Y\n", defaultYes: false, want: true},
		{name: "n selects no", input: "n\n", defaultYes: true, want: false},
		{name: "no selects no", input: "no\n", defaultYes: true, want: false},
		{name: "other input selects no", input: "maybe\n", defaultYes: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			got, err := promptBool(reader, io.Discard, "Include", tt.defaultYes)
			if err != nil {
				t.Fatalf("promptBool() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("promptBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPromptBoolNilReader(t *testing.T) {
	got, err := promptBool(nil, io.Discard, "Include", true)
	if err != nil {
		t.Fatalf("promptBool() error = %v", err)
	}
	if !got {
		t.Error("promptBool() with nil reader returned false, want default true")
	}
}

func TestPromptFeaturesSelectAll(t *testing.T) {
	gen := New()
	gen.Stdout = io.Discard

	// one "y" per non-hidden component
	input := strings.Repeat("y\n", countSelectableComponents(gen.Manifest))
	reader := bufio.NewReader(strings.NewReader(input))

	selected, err := gen.promptFeatures(reader)
	if err != nil {
		t.Fatalf("promptFeatures() error = %v", err)
	}

	for _, c := range gen.Manifest.Components {
		if c.Hidden {
			continue
		}
		if !containsID(selected, c.ID) {
			t.Errorf("promptFeatures() missing %q, got %v", c.ID, selected)
		}
	}
	if containsID(selected, FeatureCore) {
		t.Error("promptFeatures() should not return core explicitly when other features are selected")
	}
}

func TestPromptFeaturesSelectNone(t *testing.T) {
	gen := New()
	gen.Stdout = io.Discard

	// one "n" per non-hidden component
	input := strings.Repeat("n\n", countSelectableComponents(gen.Manifest))
	reader := bufio.NewReader(strings.NewReader(input))

	selected, err := gen.promptFeatures(reader)
	if err != nil {
		t.Fatalf("promptFeatures() error = %v", err)
	}

	if len(selected) != 1 || selected[0] != FeatureCore {
		t.Errorf("promptFeatures() with all-no = %v, want [%s]", selected, FeatureCore)
	}
}

func TestPromptFeaturesDefaultIncludesAll(t *testing.T) {
	gen := New()
	gen.Stdout = io.Discard

	// pressing Enter for every prompt uses the default (Y)
	input := strings.Repeat("\n", countSelectableComponents(gen.Manifest))
	reader := bufio.NewReader(strings.NewReader(input))

	selected, err := gen.promptFeatures(reader)
	if err != nil {
		t.Fatalf("promptFeatures() error = %v", err)
	}

	if len(selected) != countSelectableComponents(gen.Manifest) {
		t.Errorf("promptFeatures() with all-default = %v, want all %d selectable features", selected, countSelectableComponents(gen.Manifest))
	}
}

func TestPromptFeaturesHiddenComponentsNotPrompted(t *testing.T) {
	gen := New()
	gen.Stdout = io.Discard

	// Provide exactly enough input for selectable components only.
	// If core were prompted, we'd run short on input and get an EOF error.
	input := strings.Repeat("y\n", countSelectableComponents(gen.Manifest))
	reader := bufio.NewReader(strings.NewReader(input))

	if _, err := gen.promptFeatures(reader); err != nil {
		t.Fatalf("promptFeatures() error = %v (hidden component may have been prompted)", err)
	}
}

func countSelectableComponents(m Manifest) int {
	n := 0
	for _, c := range m.Components {
		if !c.Hidden {
			n++
		}
	}
	return n
}

func containsID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
