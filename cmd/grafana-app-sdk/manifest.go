package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/grafana/codejen"
	k8sversion "k8s.io/apimachinery/pkg/version"
)

var errNoVersions = errors.New("no versions found")

func getManifestLatestVersion(manifestDir string) (string, error) {
	// Parse the CUE
	inst := load.Instances(nil, &load.Config{
		Dir:        manifestDir,
		ModuleRoot: manifestDir,
	})
	if len(inst) == 0 {
		return "", errors.New("no data")
	}
	root := cuecontext.New().BuildInstance(inst[0])
	if root.Err() != nil {
		return "", fmt.Errorf("failed to load manifest: %w", root.Err())
	}

	// Find manifest.versions and check if the version key already exists.
	// The easiest way to do this is in the CUE, rather than the bytes
	versionsObj := root.LookupPath(cue.MakePath(cue.Str("manifest"), cue.Str("versions")))
	if versionsObj.Err() != nil {
		return "", fmt.Errorf("could not find versions field in manifest.cue: %w", versionsObj.Err())
	}
	it, err := versionsObj.Fields()
	if err != nil {
		return "", fmt.Errorf("could not get versions fields: %w", err)
	}

	versions := make([]string, 0)
	for it.Next() {
		versions = append(versions, it.Selector().String())
	}
	if len(versions) == 0 {
		return "", errNoVersions
	}
	sort.Slice(versions, func(i, j int) bool {
		return k8sversion.CompareKubeAwareVersionStrings(versions[i], versions[j]) > 0
	})
	return versions[0], nil
}

func getManifestKindsForVersion(manifestDir, version string) ([]string, error) {
	// Parse the CUE
	inst := load.Instances(nil, &load.Config{
		Dir:        manifestDir,
		ModuleRoot: manifestDir,
	})
	if len(inst) == 0 {
		return nil, errors.New("no data")
	}
	root := cuecontext.New().BuildInstance(inst[0])
	if root.Err() != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", root.Err())
	}

	// Find manifest.versions and check if the version key already exists.
	// The easiest way to do this is in the CUE, rather than the bytes
	versionsObj := root.LookupPath(cue.MakePath(cue.Str("manifest"), cue.Str("versions")))
	if versionsObj.Err() != nil {
		return nil, fmt.Errorf("could not find versions field in manifest.cue: %w", versionsObj.Err())
	}
	it, err := versionsObj.Fields()
	if err != nil {
		return nil, fmt.Errorf("could not get versions fields: %w", err)
	}
	kinds := make([]string, 0)
	for it.Next() {
		if it.Selector().String() != version {
			continue
		}
		kit, err := it.Value().LookupPath(cue.MakePath(cue.Str("kinds"))).List()
		if err != nil {
			return nil, fmt.Errorf("could not get kinds for version %s in manifest.cue: %w", version, err)
		}
		for kit.Next() {
			kind, err := kit.Value().LookupPath(cue.MakePath(cue.Str("kind"))).String()
			if err != nil {
				return nil, fmt.Errorf("could not get kind for versions[\"%s\"].kinds[%s] in manifest.cue: %w", version, kit.Selector().String(), err)
			}
			kinds = append(kinds, kind)
		}
	}
	return kinds, nil
}

//nolint:funlen
func addVersionedKindToManifestBytesCUE(manifestDir string, manifestFileName string, version string, fieldName string) (codejen.Files, error) {
	// Parse the CUE
	inst := load.Instances(nil, &load.Config{
		Dir:        manifestDir,
		ModuleRoot: manifestDir,
	})
	if len(inst) == 0 {
		return nil, errors.New("no data")
	}
	root := cuecontext.New().BuildInstance(inst[0])
	if root.Err() != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", root.Err())
	}

	// Find manifest.versions and check if the version key already exists.
	// The easiest way to do this is in the CUE, rather than the bytes
	versionsObj := root.LookupPath(cue.MakePath(cue.Str("manifest"), cue.Str("versions")))
	if versionsObj.Err() != nil {
		return nil, fmt.Errorf("could not find versions field in manifest.cue: %w", versionsObj.Err())
	}
	it, err := versionsObj.Fields()
	if err != nil {
		return nil, fmt.Errorf("could not get versions fields: %w", err)
	}

	var versionObj cue.Value
	for it.Next() {
		if it.Selector().String() == version {
			versionObj = it.Value()
			break
		}
	}

	// Load the manifest file contents
	file, err := os.DirFS(manifestDir).Open(manifestFileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	manifestBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	contents := string(manifestBytes)
	versionsMatcher := regexp.MustCompile(`(?m)^(\s*versions\s*:)(.*)$`)
	if !versionObj.Exists() {
		// No version object, add it to the map, create the version object
		// best practice would be to add it to the end, but there's not an easy way to get the end of the end of the map
		// with regex, so we instead add it to the beginning. Since maps are unsorted, this isn't  an issue with parsing.
		matches := versionsMatcher.FindStringSubmatch(contents)
		if len(matches) < 3 {
			return nil, fmt.Errorf("could not find kinds field in %s.cue", manifestFileName)
		}
		versionsStr := matches[2]
		if regexp.MustCompile(`^\s*\{`).MatchString(versionsStr) {
			// Direct map, we can just add our key at the start
			// get the remainder of the line
			restOfLine := ""
			lineMatches := regexp.MustCompile(`^(\s*\{)(.*)$`).FindStringSubmatch(versionsStr)
			if len(lineMatches) == 3 {
				restOfLine = lineMatches[2]
			}
			contents = versionsMatcher.ReplaceAllString(contents, matches[1]+" {\n\""+version+"\":"+version+"\n"+restOfLine)
		} else {
			// Not a simple list, prepend a map with our version
			contents = versionsMatcher.ReplaceAllString(contents, matches[1]+" {\""+version+"\": "+version+"} + "+matches[2])
		}

		contents = fmt.Sprintf(`%s
%s: {
	kinds: [%s]
}
`, contents, version, fieldName)

		return codejen.Files{{
			RelativePath: manifestFileName,
			Data:         []byte(contents),
		}}, nil
	}

	// If it does, check if we use a version object for it
	versionObj = root.LookupPath(cue.MakePath(cue.Str(version)))
	if versionObj.Err() != nil || !versionObj.Exists() {
		// If not, find the kinds section for the version and add our kind to the beginning of that array
		loc := versionsMatcher.FindStringIndex(contents)
		if len(loc) == 0 || loc[0] <= 0 {
			return nil, fmt.Errorf("could not find versions field in %s.cue", manifestFileName)
		}
		prev := contents[:loc[1]]
		next := contents[loc[1]:]
		loc = regexp.MustCompile(fmt.Sprintf(`(?m)^(\s*"%s"\s*:)(.*)$`, version)).FindStringIndex(next)
		if len(loc) == 0 || loc[0] <= 0 {
			return nil, fmt.Errorf("could not find versions[\"%s\"] field in %s.cue", version, manifestFileName)
		}
		prev += next[:loc[1]]
		next = next[loc[1]:]
		next, err = addToFirstKindsSection(next, fieldName)
		if err != nil {
			return nil, err
		}
		contents = prev + next

		return codejen.Files{{
			RelativePath: manifestFileName,
			Data:         []byte(contents),
		}}, nil
	}

	// Find the kinds section of the version object
	// Prepend our kind to that object
	// We use some pretty brittle regex here because modifying the CUE graph is messy and will often create changes
	// beyond the scope of what we're trying to do (and results in CUE which isn't nearly as readable).
	versionMatcher := regexp.MustCompile(fmt.Sprintf(`(?m)^(\s*%s\s*:)(.*)$`, version))
	loc := versionMatcher.FindStringIndex(contents)
	if len(loc) == 0 || loc[0] <= 0 {
		return nil, fmt.Errorf("could not find field '%s' in %s.cue", version, manifestFileName)
	}
	prev := contents[:loc[1]]
	next := contents[loc[1]:]
	next, err = addToFirstKindsSection(next, fieldName)
	if err != nil {
		return nil, err
	}
	contents = prev + next

	return codejen.Files{{
		RelativePath: manifestFileName,
		Data:         []byte(contents),
	}}, nil
}

func addToFirstKindsSection(contents string, toAdd string) (string, error) {
	expr := regexp.MustCompile(`(?m)^(\s*kinds\s*:)(.*)$`)
	matches := expr.FindStringSubmatch(contents)
	if len(matches) < 3 {
		return "", errors.New("could not find kinds field")
	}
	loc := expr.FindStringIndex(contents)
	loc0 := loc[0] + len(matches[1])
	kindsStr := matches[2]
	if regexp.MustCompile(`^\s*\[`).MatchString(kindsStr) {
		// Direct array, we can prepend our field
		// Check if there's anything in the array
		if regexp.MustCompile(`^\s\[\s*]`).MatchString(kindsStr) {
			// Empty, just replace with our field
			return contents[:loc0] + "[" + toAdd + "]" + contents[loc[1]:], nil
		}
		kindsStr = regexp.MustCompile(`^\s*\[`).ReplaceAllString(kindsStr, " ["+toAdd+", ")
		return contents[:loc0] + kindsStr + contents[loc[1]:], nil
	}
	// Not a simple list, prepend `[<fieldname>] + `
	return contents[:loc0] + "kinds: [" + toAdd + "] + " + kindsStr + contents[loc[1]:], nil
}
