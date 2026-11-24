package app

import (
	"os"
	"path/filepath"
	"testing"

	"bitbucket.org/atlassian-developers/proximity/internal/settings"
)

func TestProfileSelectionPersistsWithoutChangingOriginalSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "proximity", "settings.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("# Keep comments and other configuration\n[vars]\ndefaultProfile = 'default'\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	newApp := func() *App {
		a := &App{settingsPath: "/.config/proximity/settings", settings: &settings.Struct{Vars: map[string]any{
			"aiGatewayEnv": "staging", "defaultProfile": "default",
			"profiles": []map[string]any{{"name": "default", "useCaseId": "rad-venn-prototype"}},
		}}}
		a.initProfiles()
		return a
	}
	a := newApp()
	if got := a.GetProfileSettings(); got.Selected != "default" || len(got.Profiles) != 2 || got.Profiles[1].UseCaseID != evaluationUseCase {
		t.Fatalf("unexpected profiles: %+v", got)
	}
	if err := a.SetDefaultProfile("shared-eval"); err != nil {
		t.Fatal(err)
	}
	if got := newApp().selectedProfile(); got != "shared-eval" {
		t.Fatalf("selected profile after restart = %q", got)
	}
	if err := a.SetDefaultProfile("missing"); err == nil {
		t.Fatal("expected unknown profile error")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("settings file changed: %q, %v", got, err)
	}
}

func TestEvaluationProfileOnlyAddedInStaging(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &App{settingsPath: ".config/proximity/settings", settings: &settings.Struct{Vars: map[string]any{
		"aiGatewayEnv": "prod", "defaultProfile": "default", "profiles": []map[string]any{{"name": "default", "useCaseId": "my-use-case"}},
	}}}
	a.initProfiles()
	if profiles := a.GetProfileSettings().Profiles; len(profiles) != 1 {
		t.Fatalf("unexpected production profiles: %+v", profiles)
	}
}
