package commands

import (
	"github.com/Jumpscale/go-raml/codegen"
	"github.com/Jumpscale/go-raml/raml"

	log "github.com/Sirupsen/logrus"
)

//ClientCommand is executed to generate client from a RAML specification
type ClientCommand struct {
	Language        string
	Dir             string //target dir
	RamlFile        string //raml file
	PackageName     string //package name in the generated go source files
	ImportPath      string
	Kind            string
	PackageVersion  string //package version string
	JavaAnnotations string //optionally use Jackson or GSON annotations when generating Java code
}

//Execute generates a client from a RAML specification
func (command *ClientCommand) Execute() error {
	log.Debug("Generating a rest client for ", command.Language)
	apiDef := new(raml.APIDefinition)
	err := raml.ParseFile(command.RamlFile, apiDef)
	if err != nil {
		return err
	}

	return codegen.GenerateClient(apiDef, command.Dir, command.PackageName, command.Language, command.ImportPath,
		command.Kind, command.PackageVersion, command.JavaAnnotations)
}
