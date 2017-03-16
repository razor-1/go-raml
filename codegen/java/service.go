package java

import (
	"path/filepath"
	"strings"

	"github.com/Jumpscale/go-raml/codegen/resource"
)

type service struct {
	rootEndpoint string
	Methods      []resource.MethodInterface
}

// Name returns it's struct name
func (s service) Name() string {
	return strings.Title(s.rootEndpoint[1:]) + "Service"
}

// EndpointName returns root endpoint name
func (s service) EndpointName() string {
	return s.rootEndpoint[1:]
}

// FilenameNoExt return filename without extension
// this function is needed by template
func (s service) FilenameNoExt() string {
    return s.Name()
	// return strings.Title(s.rootEndpoint[1:]) + "Service"
}

func (s service) filename(dir string) string {
	return filepath.Join(dir, s.FilenameNoExt()) + ".java"
}
