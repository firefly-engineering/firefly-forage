package skills

import (
	"bytes"
	"embed"
	"strings"
	"text/template"
)

//go:embed templates/*.md.tmpl
var templatesFS embed.FS

var skillTemplates *template.Template

func init() {
	funcs := template.FuncMap{
		"joinStrings": strings.Join,
	}
	skillTemplates = template.Must(
		template.New("").Funcs(funcs).ParseFS(templatesFS, "templates/*.md.tmpl"),
	)
}

// renderTemplate executes a named template with the given data and returns the result.
func renderTemplate(name string, data any) string {
	var buf bytes.Buffer
	if err := skillTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		// Programming error — templates are embedded and tested at init time.
		panic("skills: failed to render template " + name + ": " + err.Error())
	}
	return buf.String()
}
