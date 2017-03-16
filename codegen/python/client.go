package python

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/codegen/resource"
	"github.com/Jumpscale/go-raml/codegen/security"
	"github.com/Jumpscale/go-raml/raml"
)

const (
	clientNameRequests = "requests"
	clientNameAiohttp  = "aiohttp"
)

var (
	globAPIDef *raml.APIDefinition
)

// Client represents a python client
type Client struct {
	Name           string
	APIDef         *raml.APIDefinition
	BaseURI        string
	Services       map[string]*service
	Kind           string
	Template       clientTemplate
	PackageName    string
	PackageVersion string
}

// NewClient creates a python Client
func NewClient(apiDef *raml.APIDefinition, kind string, packageName string, packageVersion string) Client {
	globAPIDef = apiDef

	services := map[string]*service{}
	for k, v := range apiDef.Resources {
		rd := resource.New(apiDef, commons.NormalizeURITitle(apiDef.Title), "")
		rd.GenerateMethods(&v, "python", newServerMethodFlask, newClientMethod)
		services[k] = &service{
			rootEndpoint: k,
			Methods:      rd.Methods,
		}
	}

	switch kind {
	case "":
		kind = clientNameRequests
	case clientNameRequests, clientNameAiohttp:
	default:
		log.Fatalf("invalid client kind:%v", kind)
	}

	c := Client{
		Name:           commons.NormalizeURI(apiDef.Title),
		APIDef:         apiDef,
		BaseURI:        apiDef.BaseURI,
		Services:       services,
		PackageVersion: packageVersion,
		Kind:           kind,
	}
	if strings.Index(c.BaseURI, "{version}") > 0 {
		c.BaseURI = strings.Replace(c.BaseURI, "{version}", apiDef.Version, -1)
	}
	return c
}

// generate empty __init__.py without overwrite it
func generateEmptyInitPy(dir string) error {
	return commons.GenerateFile(nil, "./templates/init_py.tmpl", "init_py", filepath.Join(dir, "__init__.py"), false)
}

func PythonSafeSingleQuotedString(str string) string {
	// replace ' with \'
	// replace \ with \\
	return strings.Replace(strings.Replace(str, "\\", `\\`, -1), "'", `\'`, -1)
}

// Generate generates python client library files
func (c Client) Generate(dir string) error {
	// use special Documentation nodes in RAML to get package metadata
	packageName := ""
	packageURL := ""
	authorName := ""
	authorEmail := ""
	for _, ramlDoc := range c.APIDef.Documentation {
		lowerTitle := strings.ToLower(ramlDoc.Title)
		if strings.HasPrefix(lowerTitle, "data.") {
			dataTitle := strings.SplitN(lowerTitle, ".", 2)[1]
			switch dataTitle {
			case "authorname":
				authorName = ramlDoc.Content
			case "authoremail":
				authorEmail = ramlDoc.Content
			case "homepage":
				packageURL = ramlDoc.Content
			case "python.package_name":
				packageName = ramlDoc.Content
			}
		}
	}

	clientName := c.Name
	classDir := dir
	if packageName != "" {
		classDir = path.Join(dir, packageName)
		if err := commons.CheckCreateDir(classDir); err != nil {
			return err
		}
		clientName = packageName
	}
	clientName += ".py"

	err, typeNames := generateClasses(c.APIDef.Types, classDir)
	if err != nil {
		log.Errorf("failed to generate python classes:%v", err)
		return err
	}

	// generate client itself
	customResourceType := false
	clientURI := ""
	getMethodTypes := make(map[string]*raml.Method)
	typeImportsMap := make(map[string]struct{})
	bodyMimeType := "application/json"
	if c.APIDef.MediaType != "" {
		bodyMimeType = c.APIDef.MediaType
	}
	// strip all non-alphanumeric characters from the type name
	re := regexp.MustCompile("[[:^alnum:]]+")
	// see if there's a type param on any method of any resource matching one of the defined types in the raml document
	// if so, set type_imports based on them, flag customResourceType, and set baseType
	// TODO handle nested resources?
	for _, resource := range c.APIDef.Resources {
		// check get method responses to see what the body of 200 is
		if resource.Get != nil {
			resType := resource.Get.Responses["200"].Bodies.Type
			resType = re.ReplaceAllLiteralString(resType, "")
			getMethodTypes[resType] = resource.Get
			clientURI = resource.URI
		}
	}

	// see if baseType is one of the types defined in this raml definition.
	// iterate over all defined types and see if one matches a type from the resource
	baseType := ""
	queryParameters := make(map[string]string)
	for typeName, _ := range c.APIDef.Types {
		if _, found := getMethodTypes[typeName]; found {
			customResourceType = true
			baseType = typeName
			typeImportsMap[baseType] = struct{}{}
			// handle query parameters
			for qpName, qpNamedParameter := range getMethodTypes[typeName].QueryParameters {
				queryParameters[qpName] = qpNamedParameter.Type
			}
		}
	}

	typeImports := make([]string, len(typeImportsMap))
	i := 0
	for k := range typeImportsMap {
		typeImports[i] = k
		i++
	}
	clientData := struct {
		Name               string
		Imports            []string
		CustomResourceType bool
		BaseURI            string
		ClientURI          string
		BaseType           string
		QueryParameters    map[string]string
		BodyMimeType       string
	}{
		baseType,
		typeImports,
		customResourceType,
		c.BaseURI,
		clientURI,
		baseType,
		queryParameters,
		bodyMimeType,
	}
	clientModule := clientData.Name + "Client"
	clientFilename := clientModule + ".py"
	if err := commons.GenerateFile(clientData, "./templates/client_v2_python.tmpl",
		"client_python", filepath.Join(classDir, clientFilename), false); err != nil {
		return err
	}

	typeNames = append(typeNames, clientModule)
	c.generateInitPy(classDir, typeNames)

	// generate helper
	if err := commons.GenerateFile(nil, "./templates/client_support_python.tmpl",
		"client_support_python", filepath.Join(classDir, "client_support.py"), false); err != nil {
		return err
	}

	// generate setup.py
	packageVersion := c.PackageVersion
	if packageVersion == "" {
		// we didn't get a package version on the CLI. generate one.
		packageVersion = commons.GeneratePkgVersion(c.APIDef.Version)
	}

	setupData := struct {
		PackageName    string
		PackageVersion string
		PackageDesc    string
		PackageURL     string
		AuthorName     string
		AuthorEmail    string
	}{
		packageName,
		packageVersion,
		PythonSafeSingleQuotedString(c.APIDef.Title),
		packageURL,
		authorName,
		authorEmail,
	}
	if err := commons.GenerateFile(setupData, "./templates/client_setup_python.tmpl",
		"setup_python", filepath.Join(dir, "setup.py"), false); err != nil {
		return err
	}

	return nil
}

func (c Client) generateServices(dir string) error {
	for _, s := range c.Services {
		sort.Sort(resource.ByEndpoint(s.Methods))
		if err := commons.GenerateFile(s, "./templates/client_service_python.tmpl", "client_service_python", s.filename(dir), false); err != nil {
			return err
		}
	}
	return nil
}

func (c Client) generateSecurity(dir string) error {
	for name, ss := range c.APIDef.SecuritySchemes {
		if !security.Supported(ss) {
			continue
		}
		ctx := map[string]string{
			"Name":           oauth2ClientName(name),
			"AccessTokenURI": fmt.Sprintf("%v", ss.Settings["accessTokenUri"]),
		}
		filename := filepath.Join(dir, oauth2ClientFilename(name))
		if err := commons.GenerateFile(ctx, "./templates/oauth2_client_python.tmpl", "oauth2_client_python", filename, true); err != nil {
			return err
		}
	}
	return nil
}

func (c Client) generateInitPy(dir string, typeNames []string) error {
	filename := filepath.Join(dir, "__init__.py")
	initData := struct {
		PackageModules []string
	}{
		typeNames,
	}
	return commons.GenerateFile(initData, "./templates/client_initpy_python_v2.tmpl", "client_initpy_python_v2", filename, false)
}
