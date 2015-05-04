package datad

func truncate(s string, maxChars int, more string) string {
	if len(s)+len(more) > maxChars {
		return s[:maxChars-len(more)] + more
	}
	return s
}
