package utils

// ContainsString check if a slice contains a specific string
func ContainsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// RemoveString remove a specifici string from a slice
func RemoveString(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}
