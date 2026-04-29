package generator

import (
	"reflect"
	"strings"
	"testing"
)

func TestManifestResolveIncludesDependencies(t *testing.T) {
	components, err := DefaultManifest().Resolve([]string{FeatureAuth})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	got := ComponentIDs(components)
	want := []string{
		FeatureAuth,
		FeatureCore,
		FeatureEmailOutbox,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ComponentIDs() = %v, want %v", got, want)
	}
}

func TestManifestResolveAllIncludesEveryComponent(t *testing.T) {
	manifest := DefaultManifest()
	components, err := manifest.Resolve([]string{FeatureAll})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(components) != len(manifest.Components) {
		t.Fatalf("Resolve(all) returned %d components, want %d", len(components), len(manifest.Components))
	}
}

func TestManifestResolveRejectsUnknownComponent(t *testing.T) {
	_, err := DefaultManifest().Resolve([]string{"billing"})
	if err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
}

func TestManifestResolveRejectsRemovedFoundationalComponents(t *testing.T) {
	for _, removed := range []string{"sqlite", "web", "csrf", "password-reset", "email-verification", "email-change"} {
		t.Run(removed, func(t *testing.T) {
			_, err := DefaultManifest().Resolve([]string{removed})
			if err == nil {
				t.Fatalf("Resolve(%q) error = nil, want error", removed)
			}
			if !strings.Contains(err.Error(), "unknown component") {
				t.Fatalf("Resolve(%q) error = %v, want unknown component error", removed, err)
			}
		})
	}
}
