package python

import (
	"path/filepath"

	log "github.com/Sirupsen/logrus"

	"github.com/Jumpscale/go-raml/codegen/commons"
	"github.com/Jumpscale/go-raml/raml"
)

// FlaskServer represents a flask server
type FlaskServer struct {
	APIDef       *raml.APIDefinition
	Title        string
	ResourcesDef []pythonResource
	WithMain     bool
	APIDocsDir   string
}

// NewFlaskServer creates new flask server from an RAML file
func NewFlaskServer(apiDef *raml.APIDefinition, apiDocsDir string, withMain bool) *FlaskServer {
	globAPIDef = apiDef

	var prs []pythonResource
	for _, rd := range getServerResourcesDefs(apiDef) {
		pr := newResource(rd, apiDef, newServerMethodFlask)
		prs = append(prs, pr)
	}

	return &FlaskServer{
		APIDef:       apiDef,
		Title:        apiDef.Title,
		APIDocsDir:   apiDocsDir,
		WithMain:     withMain,
		ResourcesDef: prs,
	}
}

// Generate generates all python server files
func (ps FlaskServer) Generate(dir string) error {
	// generate input validators helper
	if err := commons.GenerateFile(struct{}{}, "./templates/input_validators_python.tmpl", "input_validators_python",
		filepath.Join(dir, "input_validators.py"), false); err != nil {
		return err
	}

	// generate request body
	if err := ps.generateClassesFromBodies(dir); err != nil {
		return err
	}

	// python classes
	if err, typeNames := generateClasses(ps.APIDef.Types, dir); err != nil {
		log.Errorf("failed to generate python clased:%v: types%v", err, typeNames)
		return err
	}

	// security scheme
	if err := ps.generateOauth2(ps.APIDef.SecuritySchemes, dir); err != nil {
		log.Errorf("failed to generate security scheme:%v", err)
		return err
	}

	// genereate resources
	if err := ps.generateResources(dir); err != nil {
		return err
	}

	// libraries
	if err := generateLibraries(ps.APIDef.Libraries, dir); err != nil {
		return err
	}

	// requirements.txt file
	if err := commons.GenerateFile(nil, "./templates/requirements_python.tmpl", "requirements_python", filepath.Join(dir, "requirements.txt"), false); err != nil {
		return err
	}

	// generate main
	if ps.WithMain {
		// generate HTML front page
		if err := commons.GenerateFile(ps, "./templates/index.html.tmpl", "index.html", filepath.Join(dir, "index.html"), false); err != nil {
			return err
		}
		// main file
		return commons.GenerateFile(ps, "./templates/server_main_python.tmpl", "server_main_python", filepath.Join(dir, "app.py"), true)
	}
	return nil

}
