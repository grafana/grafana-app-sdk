package util

import (
	"net/url"

	"github.com/getkin/kin-openapi/openapi3"
)

func LoadSwagger(filePath string) (swagger *openapi3.T, err error) {

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	u, err := url.Parse(filePath)
	if err == nil && u.Scheme != "" && u.Host != "" {
		return loader.LoadFromURI(u)
	} else {
		return loader.LoadFromFile(filePath)
	}
}

func LoadSwaggerWithCircularReferenceCount(filePath string, _ int) (*openapi3.T, error) {
	// FYI(@radiohead):
	// github.com/getkin/kin-openapi/openapi3 has implemented reference backtracking in v0.126.0,
	// so there is no longer a need for special handling of circular references.
	return LoadSwagger(filePath)
}
