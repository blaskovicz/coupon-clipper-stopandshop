package common

import "html/template"

var Templates *template.Template

func init() {
	Templates = template.Must(template.ParseGlob("templates/*.tmpl"))
}
