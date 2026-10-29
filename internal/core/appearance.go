package core

import "fmt"

type AppearancePreferences struct {
	TextSize          string `json:"textSize"`
	HiddenCardDetails int    `json:"hiddenCardDetails"`
	CapacityParts     int    `json:"capacityParts"`
	Sidebar           string `json:"sidebar"`
	Motion            string `json:"motion"`
	DisconnectSize    string `json:"disconnectSize"`
	DisconnectDelayMS int    `json:"disconnectDelayMs"`
}

func appearanceDefaults() AppearancePreferences {
	return AppearancePreferences{"standard", 0, 7, "expanded", "full", "standard", 1000}
}
func validateAppearance(p *AppearancePreferences) error {
	if p == nil {
		return nil
	}
	oneOf := func(v string, values ...string) bool {
		for _, value := range values {
			if v == value {
				return true
			}
		}
		return false
	}
	if !oneOf(p.TextSize, "small", "standard", "large") || p.HiddenCardDetails < 0 || p.HiddenCardDetails > 15 || p.CapacityParts < 1 || p.CapacityParts > 7 || !oneOf(p.Sidebar, "expanded", "collapsed", "auto") || !oneOf(p.Motion, "full", "reduced", "off") || !oneOf(p.DisconnectSize, "compact", "standard", "large") || p.DisconnectDelayMS < 0 || p.DisconnectDelayMS > 60000 {
		return fmt.Errorf("invalid appearance preferences")
	}
	return nil
}

// DesktopAppearance exposes only cosmetic preferences, including while locked.
func DesktopAppearance(s *Service) AppearancePreferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store.Settings.Appearance != nil {
		return *s.store.Settings.Appearance
	}
	return appearanceDefaults()
}
