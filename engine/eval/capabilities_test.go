package eval

import (
	"strings"
	"testing"
)

func TestCapabilitiesManifestIsWellFormed(t *testing.T) {
	valid := map[CapabilityGroup]bool{
		CapHeadscale: true, CapTailground: true, CapExpanded: true,
		CapNotModeled: true, CapByDesign: true,
	}
	seen := map[string]bool{}
	groups := map[CapabilityGroup]int{}
	for _, c := range Capabilities {
		if c.ID == "" || c.Name == "" {
			t.Errorf("capability %+v needs an ID and a Name", c)
		}
		if seen[c.ID] {
			t.Errorf("duplicate capability ID %q", c.ID)
		}
		seen[c.ID] = true
		if !valid[c.Group] {
			t.Errorf("%s: unknown group %q", c.ID, c.Group)
		}
		groups[c.Group]++
		// The UI pairs backticks into <code> spans; an odd count would swallow text.
		for _, s := range []string{c.Name, c.Notes} {
			if strings.Count(s, "`")%2 != 0 {
				t.Errorf("%s: unbalanced backticks in %q", c.ID, s)
			}
		}
	}
	for g := range valid {
		if groups[g] == 0 {
			t.Errorf("group %q has no capabilities; the UI would hide its card", g)
		}
	}
}
