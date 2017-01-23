package python

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"regexp"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/codegen/resource"
	"github.com/Jumpscale/go-raml/codegen/security"
	"github.com/Jumpscale/go-raml/raml"
)

var (
	globAPIDef *raml.APIDefinition
)

// Client represents a python client
type Client struct {
	Name     string
	APIDef   *raml.APIDefinition
	BaseURI  string
	Services map[string]*service
}



// NewClient creates a python Client
func NewClient(apiDef *raml.APIDefinition) Client {
	services := map[string]*service{}
	for k, v := range apiDef.Resources {
		rd := resource.New(apiDef, commons.NormalizeURITitle(apiDef.Title), "")
		rd.GenerateMethods(&v, "python", newServerMethod, newClientMethod)
		services[k] = &service{
			rootEndpoint: k,
			Methods:      rd.Methods,
		}
	}
	c := Client{
		Name:     commons.NormalizeURI(apiDef.Title),
		APIDef:   apiDef,
		BaseURI:  apiDef.BaseURI,
		Services: services,
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
	if err := generateClasses(c.APIDef.Types, dir); err != nil {
		log.Errorf("failed to generate python classes:%v", err)
		return err
	}

	// generate client itself
	clientName := c.Name + "Client.py"
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
		_, found := getMethodTypes[typeName]
		if found {
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
		Name string
		Imports []string
		CustomResourceType bool
		BaseURI string
		ClientURI string
		BaseType string
		QueryParameters map[string]string
		BodyMimeType string
	} {
		c.Name,
		typeImports,
		customResourceType,
		c.BaseURI,
		clientURI,
		baseType,
		queryParameters,
		bodyMimeType,
	}
	if err := commons.GenerateFile(clientData, "./templates/client_v2_python.tmpl",
		"client_python", filepath.Join(dir, clientName), false); err != nil {
		return err
	}	

	// generate helper
	if err := commons.GenerateFile(nil, "./templates/client_support_python.tmpl",
		"client_support_python", filepath.Join(dir, "client_support.py"), false); err != nil {
		return err
	}

	// generate setup.py
	re = regexp.MustCompile("[[:space:]]")
	packageName := strings.ToLower(re.ReplaceAllLiteralString(c.APIDef.Title, "_"))
	re = regexp.MustCompile("^[^0-9]+")
	now := time.Now().UTC()
	packageVersion := fmt.Sprintf("%v.%v", re.ReplaceAllLiteralString(c.APIDef.Version, ""), now.Format("20060102.150405"))

	// use special Documentation nodes in RAML to get author and package info
	packageURL := ""
	authorName := ""
	authorEmail := ""
	for _, ramlDoc := range c.APIDef.Documentation {
		lowerTitle := strings.ToLower(ramlDoc.Title)
		if strings.HasPrefix(lowerTitle, "data.") {
			dataTitle := strings.Split(lowerTitle, ".")[1]
			switch dataTitle {
			case "authorname":
				authorName = ramlDoc.Content
				break
			case "authoremail":
				authorEmail = ramlDoc.Content
				break
			case "homepage":
				packageURL = ramlDoc.Content
				break
			}
		}
	}

	setupData := struct {
		PackageName string
		PackageVersion string
		PackageDesc string
		PackageURL string
		AuthorName string
		AuthorEmail string
	} {
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

	// if err := c.generateServices(dir); err != nil {
	// 	return err
	// }

	// if err := c.generateSecurity(dir); err != nil {
	// 	return err
	// }

	// if err := c.generateInitPy(dir); err != nil {
	// 	return err
	// }
	// // generate main client lib file
	// return commons.GenerateFile(c, "./templates/client_python.tmpl", "client_python", filepath.Join(dir, "client.py"), true)
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

func (c Client) generateInitPy(dir string) error {
	type oauth2Client struct {
		Name       string
		ModuleName string
		Filename   string
	}

	var securities []oauth2Client

	for name, ss := range c.APIDef.SecuritySchemes {
		if !security.Supported(ss) {
			continue
		}
		s := oauth2Client{
			Name:     oauth2ClientName(name),
			Filename: oauth2ClientFilename(name),
		}
		s.ModuleName = strings.TrimSuffix(s.Filename, ".py")
		securities = append(securities, s)
	}
	ctx := map[string]interface{}{
		"BaseURI":    c.APIDef.BaseURI,
		"Securities": securities,
	}
	filename := filepath.Join(dir, "__init__.py")
	return commons.GenerateFile(ctx, "./templates/client_initpy_python.tmpl", "client_initpy_python", filename, false)
}
