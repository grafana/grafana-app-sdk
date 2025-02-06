package main

import (
	"fmt"
	"regexp"
)

func addKindToManifestBytesCUE(manifestBytes []byte, kindFieldName string) ([]byte, error) {
	// Rather than attempt to load and modify in-CUE (as this is complex and will also change the CUE the user has written)
	// We will just modify the file at <kindpath>/manifest.cue and stick kindFieldName at the beginning of the `kinds` array
	// This is slightly brittle, but it keeps decent compatibility with the current `kind add` functionality.
	contents := string(manifestBytes)
	expr := regexp.MustCompile(`(?m)^(\s*kinds\s*:)(.*)$`)
	matches := expr.FindStringSubmatch(contents)
	if len(matches) < 3 {
		return nil, fmt.Errorf("could not find kinds field in manifest.cue")
	}
	kindsStr := matches[2]
	if regexp.MustCompile(`^\s*\[`).MatchString(kindsStr) {
		// Direct array, we can prepend our field
		// Check if there's anything in the array
		if regexp.MustCompile(`^\s\[\s*]`).MatchString(kindsStr) {
			// Empty, just replace with our field
			contents = expr.ReplaceAllString(contents, matches[1]+" ["+kindFieldName+"]")
		} else {
			kindsStr = regexp.MustCompile(`^\s*\[`).ReplaceAllString(kindsStr, " ["+kindFieldName+", ")
			contents = expr.ReplaceAllString(contents, matches[1]+kindsStr)
		}
	} else {
		// Not a simple list, prepend `[<fieldname>] + `
		contents = expr.ReplaceAllString(contents, matches[1]+" ["+kindFieldName+"] + "+matches[2])
	}
	return []byte(contents), nil
}
