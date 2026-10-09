package simple

import (
	"slices"
	"strings"
)

// routeTemplate is a registered custom route whose path has parameters. A
// request is sent the concrete path it was called with, so these routes are
// matched segment by segment rather than looked up by their declared path.
type routeTemplate struct {
	path     string
	segments []string
	// catchAll is set when the last segment ({name:*} or {name...}) matches the
	// rest of the request path, including any slashes.
	catchAll bool
	handler  AppCustomRouteHandler
}

// newRouteTemplate parses a declared route path. It returns false for a path
// without parameters, which is looked up by its exact value instead.
func newRouteTemplate(path string, handler AppCustomRouteHandler) (routeTemplate, bool) {
	path = strings.TrimPrefix(path, "/")
	if !strings.Contains(path, "{") {
		return routeTemplate{}, false
	}
	t := routeTemplate{path: path, segments: strings.Split(path, "/"), handler: handler}
	if last := t.segments[len(t.segments)-1]; isCatchAll(last) {
		t.catchAll = true
		t.segments = t.segments[:len(t.segments)-1]
	}
	return t, true
}

func isParameter(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func isCatchAll(segment string) bool {
	return isParameter(segment) && (strings.HasSuffix(segment, ":*}") || strings.HasSuffix(segment, "...}"))
}

// match reports whether a concrete request path matches the template. A
// parameter matches one non-empty segment; a catch-all matches whatever is left.
func (t routeTemplate) match(path string) bool {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) < len(t.segments) || (!t.catchAll && len(segments) != len(t.segments)) {
		return false
	}
	for i, want := range t.segments {
		got := segments[i]
		if isParameter(want) {
			if got == "" {
				return false
			}
			continue
		}
		if got != want {
			return false
		}
	}
	return true
}

// insertRouteTemplate adds t so that more specific templates are tried first:
// those with more literal segments, then longer ones, and catch-alls last.
func insertRouteTemplate(templates []routeTemplate, t routeTemplate) []routeTemplate {
	literals := func(t routeTemplate) int {
		n := 0
		for _, s := range t.segments {
			if !isParameter(s) {
				n++
			}
		}
		return n
	}
	i, _ := slices.BinarySearchFunc(templates, t, func(a, b routeTemplate) int {
		if a.catchAll != b.catchAll {
			if a.catchAll {
				return 1
			}
			return -1
		}
		if d := literals(b) - literals(a); d != 0 {
			return d
		}
		if d := len(b.segments) - len(a.segments); d != 0 {
			return d
		}
		return strings.Compare(a.path, b.path)
	})
	return slices.Insert(templates, i, t)
}
