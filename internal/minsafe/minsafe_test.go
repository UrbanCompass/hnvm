package minsafe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/UrbanCompass/hnvm"
)

var rules = ParseRules(hnvm.MinSafeNode)

func managed(v bool) func() (bool, bool) { return func() (bool, bool) { return v, true } }

func TestShippedRules(t *testing.T) {
	want := []Rule{{22, true, "24.22.1"}, {22, false, "22.23.4"}, {23, false, "24.22.1"}, {24, false, "24.22.1"}, {25, false, "26.12.1"}, {26, false, "26.12.1"}}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("rules = %v", rules)
	}
}

func TestSameLine(t *testing.T) {
	for _, c := range [][2]string{{"22.1.0", "22.23.4"}, {"22.23.3", "22.23.4"}, {"24.0.0", "24.22.1"}, {"26.5.0", "26.12.1"}, {"22.9.0", "22.23.4"}} {
		r := Apply(c[0], rules, Settings{})
		if r.Version != c[1] || !strings.Contains(r.Notice, "node "+c[0]+" has known security problems; using "+c[1]+" instead") {
			t.Errorf("%s: %+v", c[0], r)
		}
	}
}

func TestLeftAlone(t *testing.T) {
	for _, v := range []string{"22.23.4", "22.99.0", "22.100.0", "24.22.1", "26.12.1", "27.0.0", "30.1.2", "latest", ">=22", "22.1"} {
		if r := Apply(v, rules, Settings{}); r.Version != v || r.Notice != "" {
			t.Errorf("%s: %+v", v, r)
		}
	}
}

func TestNoFixOnLine(t *testing.T) {
	for _, c := range [][2]string{{"23.5.0", "24.22.1"}, {"25.1.0", "26.12.1"}, {"20.11.0", "24.22.1"}, {"18.12.0", "24.22.1"}, {"9.0.0", "24.22.1"}} {
		r := Apply(c[0], rules, Settings{})
		if r.Version != c[0] || !strings.Contains(r.Notice, "no fix on its line") || !strings.Contains(r.Notice, "HNVM_MIN_SAFE_NODE_MAJOR=true") {
			t.Errorf("default %s: %+v", c[0], r)
		}
		if r := Apply(c[0], rules, Settings{CrossMajor: "true"}); r.Version != c[1] {
			t.Errorf("major %s: %+v", c[0], r)
		}
	}
}

func TestOptOut(t *testing.T) {
	if r := Apply("22.1.0", rules, Settings{SameLine: "false"}); r.Version != "22.1.0" || r.Notice != "" {
		t.Errorf("%+v", r)
	}
}

func TestManaged(t *testing.T) {
	r := Apply("22.1.0", rules, Settings{SameLine: "false", ManagedSameLine: managed(true)})
	if r.Version != "22.23.4" || !strings.HasSuffix(r.Notice, "using 22.23.4 instead (set by your organization).") {
		t.Errorf("forced same-line: %+v", r)
	}
	if r := Apply("22.1.0", rules, Settings{SameLine: "true", ManagedSameLine: managed(false)}); r.Version != "22.1.0" || r.Notice != "" {
		t.Errorf("managed off: %+v", r)
	}
	if r := Apply("18.12.0", rules, Settings{ManagedMajor: managed(true)}); r.Version != "24.22.1" {
		t.Errorf("forced major: %+v", r)
	}
	r = Apply("18.12.0", rules, Settings{CrossMajor: "true", ManagedMajor: managed(false)})
	if r.Version != "18.12.0" || !strings.HasSuffix(r.Notice, "no fix on its line. Move to 24.22.1 or newer.") {
		t.Errorf("forbidden major: %+v", r)
	}
	if r := Apply("18.12.0", rules, Settings{CrossMajor: "true", ManagedSameLine: managed(false)}); r.Version != "24.22.1" {
		t.Errorf("unrelated managed key: %+v", r)
	}
}
