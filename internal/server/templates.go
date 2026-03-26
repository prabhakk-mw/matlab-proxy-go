// Copyright 2026 The MathWorks, Inc.

package server

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed all:templates
var templatesFS embed.FS

type TemplateData struct {
	BaseURL            string
	BaseURLEncoded     string // URL-encoded fully qualified base URL for the mre query parameter
	PathPrefix         string // BaseURL without trailing slash, for building asset paths
	SessionName        string
	MATLABVersion      string
	MATLABStatus       string
	BusyStatus         string
	LicensingType      string
	LicensingEmail     string
	LicensingConnStr   string
	LicensingEntitlements []EntitlementData
	AuthEnabled        bool
	ErrorMessage       string
	ErrorLogs          string
	Warnings           []string
	ShowShutdownBtn    bool
	IdleTimeout        int
	IdleTimeRemaining  int
	ConcurrencyEnabled bool
	MHLMLoginURL       string // e.g. https://login.mathworks.com/embedded-login/v2/login.html
	MHLMLoginOrigin    string // e.g. https://login.mathworks.com
}

type EntitlementData struct {
	ID    string
	Label string
}

type TemplateRenderer struct {
	templates *template.Template
}

func NewTemplateRenderer() (*TemplateRenderer, error) {
	funcMap := template.FuncMap{
		"eq": func(a, b string) bool { return a == b },
		"ne": func(a, b string) bool { return a != b },
		"gt": func(a, b int) bool { return a > b },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &TemplateRenderer{templates: tmpl}, nil
}

func (tr *TemplateRenderer) Render(w http.ResponseWriter, name string, data TemplateData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tr.templates.ExecuteTemplate(w, name+".html", data); err != nil {
		http.Error(w, "Template rendering error", http.StatusInternalServerError)
	}
}
