package atomos

import "reflect"

func UtilDiffMaps[K comparable, T any](left, right map[K]T) (added, updated, removed, changed map[K]T) {
	added = make(map[K]T)
	updated = make(map[K]T)
	removed = make(map[K]T)
	changed = make(map[K]T)

	// Find updated and removed keys
	for k, v1 := range left {
		if v2, ok := right[k]; ok {
			if !reflect.DeepEqual(v1, v2) {
				updated[k] = v2
				changed[k] = v2
			}
		} else {
			removed[k] = v1
			changed[k] = v1
		}
	}

	// Find added keys
	for k, v2 := range right {
		if _, ok := left[k]; !ok {
			added[k] = v2
			changed[k] = v2
		}
	}

	return added, updated, removed, changed
}
