package analysis

import (
	"reflect"
	"testing"
)

func TestStronglyConnectedComponentsAndCalleeFirstOrderAreDeterministic(t *testing.T) {
	nodes := []string{"a", "b", "c", "d"}
	adjacency := map[string][]string{
		"a": {"b"},
		"b": {"a", "c"},
		"c": nil,
		"d": {"a"},
	}
	components := stronglyConnectedComponents(nodes, adjacency)
	order := calleeFirstComponentOrder(components, adjacency)
	var ordered [][]string
	for _, index := range order {
		ordered = append(ordered, components[index])
	}
	want := [][]string{{"c"}, {"a", "b"}, {"d"}}
	if !reflect.DeepEqual(ordered, want) {
		t.Fatalf("orden SCC inesperado: got=%v want=%v", ordered, want)
	}
}
