package generator

import (
	"reflect"
	"testing"
)

func TestManifestResolveIncludesDependencies(t *testing.T) {
	components, err := DefaultManifest().Resolve([]string{FeatureEmailVerification})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	got := ComponentIDs(components)
	want := []string{
		FeatureAuth,
		FeatureCore,
		FeatureCSRF,
		FeatureEmailOutbox,
		FeatureEmailVerification,
		FeatureSQLite,
		FeatureWeb,
		FeatureWorker,
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
