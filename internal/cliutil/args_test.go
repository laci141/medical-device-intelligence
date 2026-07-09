package cliutil

import (
	"reflect"
	"testing"
)

func TestReorderArgs(t *testing.T) {
	vf := map[string]bool{"class": true, "limit": true}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"flags after positional", []string{"pacemaker", "--limit", "5", "--json"},
			[]string{"--limit", "5", "--json", "pacemaker"}},
		{"value flag keeps its value", []string{"pacemaker", "--class", "2"},
			[]string{"--class", "2", "pacemaker"}},
		{"equals form", []string{"pacemaker", "--class=3"},
			[]string{"--class=3", "pacemaker"}},
		{"already flags-first unchanged", []string{"--json", "pacemaker"},
			[]string{"--json", "pacemaker"}},
		{"double dash terminator", []string{"--json", "--", "--not-a-flag"},
			[]string{"--json", "--not-a-flag"}},
	}
	for _, c := range cases {
		if got := ReorderArgs(c.in, vf); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ReorderArgs(%v)=%v want %v", c.name, c.in, got, c.want)
		}
	}
}
