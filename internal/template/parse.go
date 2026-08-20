package template

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Parse decodes a qac.template/v1 YAML document into a Template.
// Unknown fields are silently ignored (forward-compat). Malformed YAML
// is reported. The returned Template is NOT validated — call Validate
// for semantic checks.
func Parse(body []byte) (Template, error) {
	var tpl Template
	if err := yaml.Unmarshal(body, &tpl); err != nil {
		return Template{}, fmt.Errorf("parse template: %w", err)
	}
	return tpl, nil
}
