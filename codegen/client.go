package codegen

import (
	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/codegen/golang"
	"github.com/Jumpscale/go-raml/codegen/java"
	"github.com/Jumpscale/go-raml/codegen/nim"
	"github.com/Jumpscale/go-raml/codegen/python"
	"github.com/Jumpscale/go-raml/raml"
)

// GenerateClient generates client library
func GenerateClient(apiDef *raml.APIDefinition, dir string, packageName string, lang string, rootImportPath string,
	kind string, packageVersion string, javaAnnotations string) error {

	if err := commons.CheckCreateDir(dir); err != nil {
		return err
	}

	switch lang {
	case langGo:
		gc, err := golang.NewClient(apiDef, packageName, rootImportPath)
		if err != nil {
			return err
		}
		return gc.Generate(dir)
	case langPython:
		pc := python.NewClient(apiDef, kind, packageName, packageVersion)
		return pc.Generate(dir)
	case langJava:
		jc := java.NewClient(apiDef, packageName, packageVersion, javaAnnotations)
		return jc.Generate(dir)
	case langNim:
		nc := nim.NewClient(apiDef, dir)
		return nc.Generate()
	default:
		return errInvalidLang
	}
}
