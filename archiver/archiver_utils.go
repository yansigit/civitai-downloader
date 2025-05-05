package archiver

import (
	"golang.org/x/exp/slices"
)

// sliceContains checks if a slice contains a specific item.
func sliceContains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}
