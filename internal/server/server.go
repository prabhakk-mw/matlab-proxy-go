// Copyright 2026 The MathWorks, Inc.

package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mathworks/matlab-proxy-go/internal/auth"
	"github.com/mathworks/matlab-proxy-go/internal/config"
	"github.com/mathworks/matlab-proxy-go/internal/licensing"
	"github.com/mathworks/matlab-proxy-go/internal/matlab"
	"github.com/mathworks/matlab-proxy-go/internal/proxy"
	"github.com/mathworks/matlab-proxy-go/internal/session"
)

//go:embed all:static
var staticFS embed.FS

// Server is the main HTTP server for matlab-proxy.
type Server struct {
	cfg        *config.Config
	auth       *auth.TokenAuth
	matlab     *matlab.Process
	licensing  *licensing.Manager
	session    *session.Manager
	httpProxy  *proxy.HTTPProxy
	wsProxy    *proxy.WebSocketProxy
	templates  *TemplateRenderer
	httpServer *http.Server
	logger     *slog.Logger
	shutdownCh chan struct{}
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	tokenAuth, err := auth.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("initializing auth: %w", err)
	}

	matlabProc := matlab.NewProcess(cfg, logger.With("component", "matlab"))
	licenseMgr := licensing.NewManager(cfg)
	sessionMgr := session.NewManager(cfg.IdleTimeoutMinutes, logger.With("component", "session"))

	tmpl, err := NewTemplateRenderer()
	if err != nil {
		return nil, fmt.Errorf("initializing templates: %w", err)
	}

	s := &Server{
		cfg:        cfg,
		auth:       tokenAuth,
		matlab:     matlabProc,
		licensing:  licenseMgr,
		session:    sessionMgr,
		httpProxy:  proxy.NewHTTPProxy(logger.With("component", "proxy")),
		wsProxy:    proxy.NewWebSocketProxy(logger.With("component", "websocket")),
		templates:  tmpl,
		logger:     logger,
		shutdownCh: make(chan struct{}),
	}

	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	// Initialize licensing
	if err := s.licensing.Init(); err != nil {
		s.logger.Warn("licensing initialization error", "error", err)
	}

	// If already licensed, start MATLAB
	if s.licensing.IsLicensed() {
		// Set up MHLM env vars before starting (access token is short-lived)
		if err := s.prepareMATLABEnv(); err != nil {
			s.logger.Error("failed to prepare MATLAB environment", "error", err)
		} else {
			go func() {
				if err := s.matlab.Start(false); err != nil {
					s.logger.Error("failed to start MATLAB", "error", err)
				}
			}()
		}
	}

	router := s.setupRoutes()

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.httpServer = &http.Server{
		Addr:           addr,
		Handler:        router,
		MaxHeaderBytes: config.MaxHTTPRequestSize,
	}

	if s.cfg.TLSConfig != nil {
		s.httpServer.TLSConfig = s.cfg.TLSConfig
	}

	// Print access information
	serverURL := s.cfg.ServerURL()
	accessURL := s.auth.AccessURL(serverURL)
	s.logger.Info("matlab-proxy is ready",
		"url", serverURL,
		"access_url", accessURL,
		"port", s.cfg.Port,
	)
	fmt.Println()
	fmt.Println("==========================================================================")
	fmt.Println("                          Access MATLAB at:")
	fmt.Printf("  %s\n", accessURL)
	fmt.Println("==========================================================================")
	fmt.Println()

	// Write server info file for discovery by list-servers
	if err := s.writeServerInfoFile(); err != nil {
		s.logger.Warn("failed to write server info file", "error", err)
	}

	// Watch for idle timeout shutdown
	go func() {
		select {
		case <-s.session.ShutdownCh():
			s.logger.Info("idle timeout triggered shutdown")
			_ = s.Shutdown(context.Background())
		case <-ctx.Done():
		}
	}()

	// Start HTTP server
	var serverErr error
	if s.cfg.TLSConfig != nil {
		// TLS certs are already in TLSConfig
		s.httpServer.TLSConfig = s.cfg.TLSConfig
		serverErr = s.httpServer.ListenAndServeTLS("", "")
	} else {
		serverErr = s.httpServer.ListenAndServe()
	}

	if serverErr == http.ErrServerClosed {
		return nil
	}
	return serverErr
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down server")

	// Clean up server info file
	s.removeServerInfoFile()

	// Stop MATLAB gracefully first (sends exit via EC), then force if needed
	if err := s.matlab.Stop(false); err != nil {
		s.logger.Warn("graceful MATLAB stop failed, forcing", "error", err)
		_ = s.matlab.Stop(true)
	}

	// Clean up any remaining MATLAB session files (connector.securePort, scripts, etc.)
	s.matlab.CleanupLogsDir()

	// Then stop HTTP server
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(shutdownCtx)
}

func (s *Server) setupRoutes() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	if s.cfg.EnableWebLogging {
		r.Use(middleware.Logger)
	}
	r.Use(s.customHeadersMiddleware)
	r.Use(s.auth.ClearStaleCookieMiddleware)

	base := s.cfg.BaseURL

	mountRoutes := func(r chi.Router) {
		// Public endpoints (no auth required)
		r.Get("/get_status", s.handleGetStatus)
		r.Post("/authenticate", s.handleAuthenticate)
		r.Get("/get_env_config", s.handleGetEnvConfig)

		// Auth-protected endpoints
		r.Group(func(r chi.Router) {
			r.Use(s.auth.Middleware)

			r.Get("/get_auth_token", s.handleGetAuthToken)
			r.Put("/start_matlab", s.handleStartMATLAB)
			r.Delete("/stop_matlab", s.handleStopMATLAB)
			r.Put("/set_licensing_info", s.handleSetLicensing)
			r.Put("/update_entitlement", s.handleUpdateEntitlement)
			r.Delete("/set_licensing_info", s.handleDeleteLicensing)
			r.Delete("/shutdown_integration", s.handleShutdown)
			r.Post("/clear_client_id", s.handleClearClientID)
		})

		// Serve the frontend UI
		r.Get("/", s.handleIndex)

		// Static files
		staticContent, _ := fs.Sub(staticFS, "static")
		fileServer := http.FileServer(http.FS(staticContent))
		stripPrefix := base + "/static/"
		if base == "/" {
			stripPrefix = "/static/"
		}
		r.Handle("/static/*", http.StripPrefix(stripPrefix, fileServer))

		// Catch-all: proxy to MATLAB Embedded Connector (auth required)
		r.HandleFunc("/*", s.handleMATLABProxy)
	}

	if base == "/" {
		mountRoutes(r)
	} else {
		r.Route(base, mountRoutes)
	}

	return r
}

func (s *Server) customHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, val := range s.cfg.CustomHTTPHeaders {
			w.Header().Set(key, val)
		}
		next.ServeHTTP(w, r)
	})
}

// --- Handlers ---

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.session.ResetIdleTimer()

	// If the user arrived with a valid token, set a session cookie
	// so iframe subrequests are authenticated automatically
	if s.auth.ValidateRequest(r) {
		s.auth.SetSessionCookie(w)
	}

	data := s.buildTemplateData(r)
	s.templates.Render(w, "index", data)
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	s.session.ResetIdleTimer()

	clientID := r.URL.Query().Get("MWI_CLIENT_ID")
	transferSession := r.URL.Query().Get("TRANSFER_SESSION") == "true"

	isActive := true
	if clientID != "" {
		isActive = s.session.SetActiveClient(clientID, transferSession)
	}

	licInfo := s.licensing.GetInfo()
	matlabStatus := s.matlab.Status()
	busyStatus := s.matlab.BusyStatus()
	errors := s.matlab.Errors()
	warnings := s.matlab.Warnings()

	var errResp interface{}
	if len(errors) > 0 {
		errResp = errors[0]
	}

	var busyStr interface{}
	if busyStatus != nil {
		busyStr = string(*busyStatus)
	}

	resp := map[string]interface{}{
		"matlab": map[string]interface{}{
			"status":     string(matlabStatus),
			"busyStatus": busyStr,
			"version":    s.cfg.MATLABVersion,
		},
		"licensing":      licInfo,
		"error":          errResp,
		"warnings":       warnings,
		"wsEnv":          "prod",
		"clientId":       clientID,
		"isActiveClient": isActive,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAuthenticate(w http.ResponseWriter, r *http.Request) {
	valid := !s.auth.Enabled() || s.auth.ValidateRequest(r)

	// Set session cookie on successful auth so subsequent requests
	// (including iframe subrequests) are authenticated automatically
	if valid {
		s.auth.SetSessionCookie(w)
	}

	resp := map[string]interface{}{
		"authentication": map[string]interface{}{
			"status": valid,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetAuthToken(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": s.auth.Token(),
	})
}

func (s *Server) handleGetEnvConfig(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"authentication": map[string]interface{}{
			"enabled": s.auth.Enabled(),
			"status":  false,
		},
		"matlab": map[string]interface{}{
			"versionOnPath":     s.cfg.MATLABVersion,
			"supportedVersions": supportedVersions(),
		},
		"browserTitle":                   s.cfg.SessionName,
		"extension_name":                 "default_configuration_matlab_proxy",
		"extension_name_short_description": "MATLAB Desktop",
		"doc_url":                        "https://github.com/mathworks/matlab-proxy",
		"should_show_shutdown_button":    true,
		"isConcurrencyEnabled":           s.session.ConcurrencyEnabled(),
		"idleTimeoutDuration":            s.cfg.IdleTimeoutMinutes * 60,
	}
	writeJSON(w, http.StatusOK, resp)
}

// prepareMATLABEnv sets up extra environment variables needed by MATLAB
// based on the current licensing type (e.g. MHLM access token).
func (s *Server) prepareMATLABEnv() error {
	licInfo := s.licensing.GetInfo()
	if licInfo.Type == licensing.TypeMHLM {
		mhlmEnv, err := s.licensing.MHLMEnvVars()
		if err != nil {
			return fmt.Errorf("fetching MHLM env vars: %w", err)
		}
		s.matlab.SetExtraEnv(mhlmEnv)
	} else {
		s.matlab.SetExtraEnv(nil)
	}
	return nil
}

func (s *Server) handleStartMATLAB(w http.ResponseWriter, r *http.Request) {
	if !s.licensing.IsLicensed() {
		http.Error(w, "MATLAB is not licensed", http.StatusBadRequest)
		return
	}

	if err := s.prepareMATLABEnv(); err != nil {
		s.logger.Error("failed to prepare MATLAB environment", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("MHLM licensing error: %v", err),
		})
		return
	}

	go func() {
		if err := s.matlab.Start(true); err != nil {
			s.logger.Error("failed to start MATLAB", "error", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "starting"})
}

func (s *Server) handleStopMATLAB(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := s.matlab.Stop(false); err != nil {
			s.logger.Error("failed to stop MATLAB", "error", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (s *Server) handleSetLicensing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type            string `json:"type"`
		ConnectionStr   string `json:"connectionString,omitempty"`
		IdentityToken   string `json:"token,omitempty"`
		SourceID        string `json:"sourceId,omitempty"`
		EmailAddr       string `json:"emailAddress,omitempty"`
		MATLABVersion   string `json:"matlabVersion,omitempty"`
		ProfileID       string `json:"profileId,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	licType := licensing.Type(req.Type)

	switch licType {
	case licensing.TypeMHLM:
		s.logger.Info("processing MHLM licensing", "email", req.EmailAddr, "hasToken", req.IdentityToken != "")
		if err := s.licensing.SetMHLMLicensing(req.IdentityToken, req.SourceID, req.EmailAddr); err != nil {
			s.logger.Error("MHLM licensing failed", "error", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.logger.Info("MHLM licensing succeeded")
	case licensing.TypeNLM:
		info := &licensing.Info{Type: licType, ConnectionStr: req.ConnectionStr}
		if err := s.licensing.SetLicensing(info); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.cfg.NLMConnStr = info.ConnectionStr
	case licensing.TypeExisting:
		if err := s.licensing.SetLicensing(&licensing.Info{Type: licType}); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown licensing type"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"licensing": s.licensing.GetInfo(),
	})
}

func (s *Server) handleUpdateEntitlement(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type          string `json:"type"`
		EntitlementID string `json:"entitlement_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.licensing.UpdateEntitlement(req.EntitlementID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"licensing": s.licensing.GetInfo(),
	})
}

func (s *Server) handleDeleteLicensing(w http.ResponseWriter, r *http.Request) {
	if err := s.licensing.ClearLicensing(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Stop MATLAB since licensing is removed
	go func() {
		if err := s.matlab.Stop(false); err != nil {
			s.logger.Error("error stopping MATLAB after license removal", "error", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"licensing": s.licensing.GetInfo(),
	})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting_down"})

	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = s.Shutdown(context.Background())
	}()
}

func (s *Server) handleClearClientID(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Also check query params (sent as beacon)
		req.ClientID = r.URL.Query().Get("MWI_CLIENT_ID")
	}
	if req.ClientID != "" {
		s.session.ClearClient(req.ClientID)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMATLABProxy(w http.ResponseWriter, r *http.Request) {
	// Authenticate first
	if s.auth.Enabled() && !s.auth.ValidateRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Refresh session cookie on successful auth
	s.auth.SetSessionCookie(w)
	s.session.ResetIdleTimer()

	ec := s.matlab.Connector()
	if ec == nil {
		// MATLAB not ready, serve the UI instead for non-API paths
		if !strings.Contains(r.URL.Path, "messageservice") {
			s.handleIndex(w, r)
			return
		}
		http.Error(w, "MATLAB is not running", http.StatusServiceUnavailable)
		return
	}

	if proxy.IsWebSocketUpgrade(r) {
		if err := s.wsProxy.Handle(w, r, ec.Port(), ec.MWAPIKey()); err != nil {
			s.logger.Error("WebSocket proxy error", "error", err)
		}
		return
	}

	s.httpProxy.Forward(w, r, ec.Port(), ec.MWAPIKey())
}

func (s *Server) buildTemplateData(r *http.Request) TemplateData {
	licInfo := s.licensing.GetInfo()
	matlabStatus := s.matlab.Status()
	busyStatus := s.matlab.BusyStatus()
	errors := s.matlab.Errors()
	warnings := s.matlab.Warnings()

	var busyStr string
	if busyStatus != nil {
		busyStr = string(*busyStatus)
	}

	var errorMsg string
	if len(errors) > 0 {
		errorMsg = errors[0].Message
	}

	pathPrefix := strings.TrimRight(s.cfg.BaseURL, "/")

	mhlmOrigin := mhlmLoginOrigin()

	return TemplateData{
		BaseURL:           s.cfg.BaseURL,
		PathPrefix:        pathPrefix,
		SessionName:       s.cfg.SessionName,
		MATLABVersion:     s.cfg.MATLABVersion,
		MATLABStatus:      string(matlabStatus),
		BusyStatus:        busyStr,
		LicensingType:     string(licInfo.Type),
		LicensingEmail:    licInfo.EmailAddr,
		LicensingConnStr:  licInfo.ConnectionStr,
		AuthEnabled:       s.auth.Enabled(),
		ErrorMessage:      errorMsg,
		Warnings:          warnings,
		ShowShutdownBtn:   true,
		IdleTimeout:       s.cfg.IdleTimeoutMinutes * 60,
		IdleTimeRemaining: s.session.IdleTimeRemaining(),
		ConcurrencyEnabled: s.session.ConcurrencyEnabled(),
		MHLMLoginURL:      mhlmOrigin + "/embedded-login/v2/login.html",
		MHLMLoginOrigin:   mhlmOrigin,
	}
}

func mhlmLoginOrigin() string {
	wsEnv := strings.ToLower(os.Getenv("WS_ENV"))
	subdomain := "login"
	if strings.Contains(wsEnv, "integ") {
		subdomain = "login-" + wsEnv
	}
	return "https://" + subdomain + ".mathworks.com"
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func supportedVersions() []string {
	return []string{
		"R2020b", "R2021a", "R2021b", "R2022a", "R2022b",
		"R2023a", "R2023b", "R2024a", "R2024b", "R2025a", "R2025b",
	}
}
