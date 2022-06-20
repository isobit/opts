package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"text/template"
)

var ErrHelp = fmt.Errorf("cli: help requested")

var usageTemplateString = `{{.Name}}{{if .Fields}} [OPTIONS]{{end}}{{if and .Commands (not .Command)}} <COMMAND>{{end}}`
var helpTemplateString = `
{{- if .Help -}}
{{.Help}}

{{end -}}
USAGE:
    {{.Usage}}

{{- if .Fields}}

OPTIONS:
{{- range .Fields}}{{if not .Hidden}}
\t    \t
{{- if .ShortName}}-{{.ShortName}}, {{end}}--{{.Name}}
{{- if .HasArg}} <{{if .Placeholder}}{{.Placeholder}}{{else}}VALUE{{end}}>{{end}}\t
{{- if .EnvVarName}}  {{.EnvVarName}}{{end}}\t
{{- if .Help}}  {{.Help}}{{end}}
{{- if and .HasArg }}{{if and .Default (not .Required)}}  (default: {{.Default}}){{else if .Required}}  (required){{end}}{{end}}
{{- end}}

{{- end}}{{end}}

{{- if .Commands}}

COMMANDS:
{{- range .Commands}}
\t    \t{{.Name}}\t
{{- if .ShortHelp}}  {{.ShortHelp}}{{end}}
{{- end}}

{{- end}}

`

var helpTemplate *template.Template
var usageTemplate *template.Template

func init() {
	helpTemplate = template.Must(
		template.New("help").Parse(helpTemplateString),
	)
	usageTemplate = template.Must(
		template.New("usage").Parse(usageTemplateString),
	)
}

func (cmd *Command) usage(command string) string {
	commands := []*Command{}
	for _, cmd := range cmd.commands {
		commands = append(commands, cmd)
	}
	data := struct {
		Name     string
		Fields   []field
		Commands []*Command
		Command  string
	}{
		Name:     cmd.Name,
		Fields:   cmd.fields,
		Commands: commands,
		Command:  command,
	}

	sb := strings.Builder{}
	if cmd.parent != nil {
		sb.WriteString(cmd.parent.usage(cmd.Name))
		sb.WriteString(" ")
	}
	err := usageTemplate.Execute(&sb, data)
	if err != nil {
		panic(fmt.Sprintf("cli: error executing usage template: %s", err))
	}
	return sb.String()
}

func (cmd *Command) HelpString() string {
	sb := strings.Builder{}
	cmd.WriteHelp(&sb)
	return sb.String()
}

func (cmd *Command) WriteHelp(w io.Writer) {
	commands := []*Command{}
	for _, cmd := range cmd.commands {
		commands = append(commands, cmd)
	}
	data := struct {
		Usage    string
		Help     string
		Fields   []field
		Commands []*Command
	}{
		Usage:    cmd.usage(""),
		Help:     cmd.Help,
		Fields:   cmd.fields, // for now all fields are flags (will impl args later)
		Commands: commands,
	}

	tw := newEscapedTabWriter(w)
	err := helpTemplate.Execute(tw, data)
	if err != nil {
		panic(fmt.Sprintf("cli: error executing help template: %s", err))
	}
	tw.Flush()
}

type escapedTabWriter struct {
	replacer  *strings.Replacer
	tabWriter *tabwriter.Writer
}

func newEscapedTabWriter(w io.Writer) escapedTabWriter {
	return escapedTabWriter{
		replacer:  strings.NewReplacer(`\t`, "\t", `\f`, "\f"),
		tabWriter: tabwriter.NewWriter(w, 0, 0, 0, ' ', 0),
	}
}

func (w escapedTabWriter) Write(p []byte) (int, error) {
	return w.replacer.WriteString(w.tabWriter, string(p))
}

func (w escapedTabWriter) Flush() error {
	return w.tabWriter.Flush()
}
