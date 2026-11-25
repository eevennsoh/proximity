package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const evaluationUseCase = "ai-gateway-eval-use-case"
const automaticProfile = "__automatic_eligible__"

type Profile struct {
	Name      string `json:"name"`
	UseCaseID string `json:"useCaseId"`
	Label     string `json:"label,omitempty"`
}

type ProfileSettings struct {
	Selected string    `json:"selected"`
	Profiles []Profile `json:"profiles"`
}

func (a *App) initProfiles() {
	if a.settings.Vars == nil {
		a.settings.Vars = make(map[string]any)
	}
	var profiles []map[string]any
	if raw, err := json.Marshal(a.settings.Vars["profiles"]); err == nil {
		_ = json.Unmarshal(raw, &profiles)
	}
	env, _ := a.settings.Vars["aiGatewayEnv"].(string)
	if env == "" || env == "staging" {
		found := false
		used := make(map[string]bool)
		for _, p := range profiles {
			name, _ := p["name"].(string)
			used[name] = true
			useCase, _ := p["useCaseId"].(string)
			cloudID, _ := p["atlassianCloudId"].(string)
			group, _ := p["adGroup"].(string)
			if useCase == evaluationUseCase && cloudID == "internal-dummy-"+evaluationUseCase && group == "ai-gateway-evaluation-dl-all-atlassian-read" {
				found = true
			}
		}
		if !found {
			name := "shared-eval"
			for n := 2; used[name]; n++ {
				name = fmt.Sprintf("shared-eval-%d", n)
			}
			profiles = append(profiles, map[string]any{
				"name": name, "useCaseId": evaluationUseCase,
				"atlassianCloudId": "internal-dummy-" + evaluationUseCase,
				"adGroup":          "ai-gateway-evaluation-dl-all-atlassian-read",
			})
		}
	}
	a.settings.Vars["profiles"] = profiles
	selected, _ := a.settings.Vars["defaultProfile"].(string)
	if selected == "" && len(profiles) > 0 {
		selected, _ = profiles[0]["name"].(string)
	}
	// Automatic routing is available only when a configured primary profile and
	// the explicitly configured staging evaluation profile are both present.
	var primary, evaluation string
	reserved := false
	for _, p := range profiles {
		name, _ := p["name"].(string)
		useCase, _ := p["useCaseId"].(string)
		cloudID, _ := p["atlassianCloudId"].(string)
		group, _ := p["adGroup"].(string)
		if name == automaticProfile {
			reserved = true
		}
		if name == selected && useCase != "" && useCase != evaluationUseCase {
			primary = name
		}
		if useCase == evaluationUseCase && cloudID == "internal-dummy-"+evaluationUseCase && group == "ai-gateway-evaluation-dl-all-atlassian-read" {
			evaluation = name
		}
	}
	if (env == "" || env == "staging") && !reserved && primary != "" && evaluation != "" {
		a.autoProfiles = []string{primary, evaluation}
	}
	if path, err := a.profilePreferencePath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var preference struct {
				Selected string `json:"selected"`
			}
			if json.Unmarshal(data, &preference) == nil && a.hasProfile(preference.Selected) {
				selected = preference.Selected
			}
		}
	}
	a.profileState.Store(selected)
}

func (a *App) profilePreferencePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, a.settingsPath+".profile.json"), nil
}

func (a *App) hasProfile(name string) bool {
	if name == automaticProfile && len(a.autoProfiles) == 2 {
		return true
	}
	for _, p := range a.profileList() {
		if p.Name == name && name != "" {
			return true
		}
	}
	return false
}

func (a *App) profileList() []Profile {
	result := []Profile{}
	profiles, _ := a.settings.Vars["profiles"].([]map[string]any)
	for _, p := range profiles {
		name, _ := p["name"].(string)
		useCase, _ := p["useCaseId"].(string)
		if name != "" {
			result = append(result, Profile{Name: name, UseCaseID: useCase})
		}
	}
	return result
}

func (a *App) selectedProfile() string {
	if value := a.profileState.Load(); value != nil {
		return value.(string)
	}
	return ""
}

func (a *App) profileForRequest() string {
	selected := a.selectedProfile()
	if selected == automaticProfile && len(a.autoProfiles) == 2 {
		return a.autoProfiles[(a.profileCycle.Add(1)-1)%2]
	}
	return selected
}

func (a *App) GetProfileSettings() ProfileSettings {
	profiles := a.profileList()
	if len(a.autoProfiles) == 2 {
		profiles = append(profiles, Profile{Name: automaticProfile, Label: "Automatic · both use cases (eligible traffic only)"})
	}
	return ProfileSettings{Selected: a.selectedProfile(), Profiles: profiles}
}

func (a *App) SetDefaultProfile(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.hasProfile(name) {
		return fmt.Errorf("unknown profile %q", name)
	}
	path, err := a.profilePreferencePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Selected string `json:"selected"`
	}{Selected: name})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".profile-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	a.profileState.Store(name)
	return nil
}
