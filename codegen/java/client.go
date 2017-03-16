package java

import (
	// "fmt"
	// "path/filepath"
	"path"
	// "sort"
	"strings"
	// "regexp"
	// "time"

	log "github.com/Sirupsen/logrus"

	"github.com/Jumpscale/go-raml/codegen/commons"
	// "github.com/Jumpscale/go-raml/codegen/resource"
	"github.com/Jumpscale/go-raml/raml"
)

var (
	globAPIDef *raml.APIDefinition
)

const (
	annotationJackson = "jackson"
	annotationGSON    = "gson"
)

// Client represents a java client
type Client struct {
	Name     string
	APIDef   *raml.APIDefinition
	BaseURI  string
	Services map[string]*service
	PackageName string
	PackageVersion string
	Annotations string
}

type templateInfo struct {
	Name 	string
	Filename string
}

// NewClient creates a java Client
func NewClient(apiDef *raml.APIDefinition, packageName string, packageVersion string, annotations string) Client {
	globAPIDef = apiDef
	services := map[string]*service{}

	ramlPackageName := getRamlTitleData("java.package_name")
	if packageName == "" && ramlPackageName != "" {
		packageName = ramlPackageName
	}

	// packageName cannot contain hyphens
	packageName = strings.Replace(packageName, "-", "_", -1)

	c := Client{
		Name:     commons.NormalizeURI(apiDef.Title),
		APIDef:   apiDef,
		BaseURI:  apiDef.BaseURI,
		Services: services,
		PackageName: packageName,
		PackageVersion: packageVersion,
	}

	annotations = strings.ToLower(annotations)
	switch annotations {
		case annotationJackson, annotationGSON:
			c.Annotations = annotations
	}

	if strings.Index(c.BaseURI, "{version}") > 0 {
		c.BaseURI = strings.Replace(c.BaseURI, "{version}", apiDef.Version, -1)
	}

	return c
}


func getRamlTitleData(title string) string {
	// see if the RAML has a title with data.{title} and if so return it
	// this allows us to put metadata into the RAML directly for things like the package name

	for _, ramlDoc := range globAPIDef.Documentation {
		lowerTitle := strings.ToLower(ramlDoc.Title)
		if strings.HasPrefix(lowerTitle, "data.") {
			dataTitle := strings.SplitN(lowerTitle, ".", 2)[1]
			if dataTitle == title {
				return ramlDoc.Content
			}
		}
	}

	return ""
}

// Generate generates java client library files
func (c Client) Generate(dir string) error {

	// build the java output path based on the package name
	packagePath := strings.Replace(c.PackageName, ".", "/", -1)
	javaDir := path.Join(dir, "src/main/java", packagePath)
	if err := commons.CheckCreateDir(javaDir); err != nil {
		return err
	}

	if err := generateClasses(javaDir, c); err != nil {
		log.Errorf("failed to generate java classes:%v", err)
		return err
	}

	// add the java validation classes
	validationTemplates := []templateInfo{
		{"validateable_java", "Validateable.java"},
		{"validation_error_java", "ValidationError.java"},
		{"validation_exception_java", "ValidationException.java"},
	}
	validationData := struct {
		PackageName string
	} { c.PackageName, }
	for _, validationTemplate := range validationTemplates {
		if err := commons.GenerateFile(validationData, path.Join("./templates/", validationTemplate.Filename)+".tmpl",
			validationTemplate.Name, path.Join(javaDir, validationTemplate.Filename), false); err != nil {
			return err
		}
	}

	// for GSON, generate InstantConverter
	if c.Annotations == annotationGSON {
		instantData := struct {
			PackageName string
		} { c.PackageName, }
		if err := commons.GenerateFile(instantData, "./templates/InstantConverter.gson.java.tmpl",
			"instantconverter_gson_java", path.Join(javaDir, "InstantConverter.java"), false); err != nil {
			return err
		}
	}

	// generate pom.xml
	packageVersion := c.PackageVersion
	if packageVersion == "" {
		// didn't get a package version from the CLI. Generate one
		packageVersion = commons.GeneratePkgVersion(c.APIDef.Version)
	}

	// derive artifactId and groupId from PackageName
	pkgComponents := strings.Split(c.PackageName, ".")
	artifactId := ""
	groupId := ""
	if len(pkgComponents) > 1 {
		artifactId = pkgComponents[len(pkgComponents)-1]
		groupId = strings.Join(pkgComponents[:len(pkgComponents)-1], ".")
	}

	ramlArtifactId := getRamlTitleData("java.artifact_id")
	if ramlArtifactId != "" {
		artifactId = ramlArtifactId
	}

	pomData := struct {
		AppName string
		ArtifactId string
		ParentArtifactId string
		GroupId string
		PackageVersion string
		Jackson bool
		GSON bool
	} {
		AppName: c.APIDef.Title,
		GroupId: groupId,
		PackageVersion: packageVersion,
	}
	switch c.Annotations {
	case annotationJackson:
		pomData.Jackson = true
		artifactId += "-jackson"
	case annotationGSON:
		pomData.GSON = true
		artifactId += "-gson"
	}
	pomData.ArtifactId = artifactId
	ramlParentArtifactSuffix := getRamlTitleData("java.parent_artifact_suffix")
	if ramlParentArtifactSuffix != "" && ramlArtifactId != "" {
		pomData.ParentArtifactId = ramlArtifactId + "-" + ramlParentArtifactSuffix
	}
	if err := commons.GenerateFile(pomData, "./templates/pom.xml.tmpl",
		"pom_xml_java", path.Join(dir, "pom.xml"), false); err != nil {
		return err
	}

	return nil
}


