package atomos

import (
	"reflect"
	"testing"
)

func TestUtilDiffMaps(t *testing.T) {
	map1 := map[string]interface{}{
		"key1": 1,
		"key2": 2,
		"key3": 3,
		"key4": 4,
	}
	map2 := map[string]interface{}{
		"key1": 1,
		"key2": 2,
		"key3": 4,
		"key5": 5,
	}
	added, updated, removed, changed := UtilDiffMaps(map1, map2)
	if !reflect.DeepEqual(changed, map[string]interface{}{
		"key3": 4,
		"key4": 4,
		"key5": 5,
	}) {
		t.Fatal("changed")
	}
	if !reflect.DeepEqual(added, map[string]interface{}{
		"key5": 5,
	}) {
		t.Fatal("added")
	}
	if !reflect.DeepEqual(updated, map[string]interface{}{
		"key3": 4,
	}) {
		t.Fatal("updated")
	}
	if !reflect.DeepEqual(removed, map[string]interface{}{
		"key4": 4,
	}) {
		t.Fatal("removed")
	}
}
