// Copyright 2026 The MathWorks, Inc.

package config

import (
	"os"
	"strconv"
	"strings"
)

// Environment variable names used by matlab-proxy.
const (
	EnvAppPort             = "MWI_APP_PORT"
	EnvAppHost             = "MWI_APP_HOST"
	EnvBaseURL             = "MWI_BASE_URL"
	EnvCustomMATLABRoot    = "MWI_CUSTOM_MATLAB_ROOT"
	EnvMLMLicenseFile      = "MLM_LICENSE_FILE"
	EnvUseExistingLicense  = "MWI_USE_EXISTING_LICENSE"
	EnvMATLABStartupScript = "MWI_MATLAB_STARTUP_SCRIPT"
	EnvEnableTokenAuth     = "MWI_ENABLE_TOKEN_AUTH"
	EnvAuthToken           = "MWI_AUTH_TOKEN"
	EnvEnableSSL           = "MWI_ENABLE_SSL"
	EnvSSLCertFile         = "MWI_SSL_CERT_FILE"
	EnvSSLKeyFile          = "MWI_SSL_KEY_FILE"
	EnvLogLevel            = "MWI_LOG_LEVEL"
	EnvLogFile             = "MWI_LOG_FILE"
	EnvEnableWebLogging    = "MWI_ENABLE_WEB_LOGGING"
	EnvCustomHTTPHeaders   = "MWI_CUSTOM_HTTP_HEADERS"
	EnvProcessStartTimeout = "MWI_PROCESS_START_TIMEOUT"
	EnvIdleTimeout         = "MWI_SHUTDOWN_ON_IDLE_TIMEOUT"
	EnvSessionName         = "MWI_SESSION_NAME"
	EnvEnableSimulink      = "MWI_ENABLE_SIMULINK"
	EnvProfileStartup      = "MWI_PROFILE_MATLAB_STARTUP"
	EnvLicModeOverride     = "MWI_LICMODE_OVERRIDE"

	// Attach mode — connect to an existing MATLAB Embedded Connector
	EnvAttachECPort   = "MWI_ATTACH_EC_PORT"
	EnvAttachMWAPIKey = "MWI_ATTACH_MWAPIKEY"
)

func GetEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func GetEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return strings.EqualFold(v, "true") || v == "1"
}

func GetEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
