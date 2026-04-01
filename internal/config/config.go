// Copyright 2026 The MathWorks, Inc.

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultHost                = "0.0.0.0"
	DefaultProcessStartTimeout = 600 // seconds
	MaxHTTPRequestSize         = 500 * 1024 * 1024
	MaxWebSocketMessageSize    = 500 * 1024 * 1024
	StatusPollInterval         = 1 * time.Second
	ConnectorSecurePortFile    = "connector.securePort"
)

type Config struct {
	Host                string
	Port                int
	BaseURL             string
	MATLABRoot          string
	MATLABVersion       string
	ProcessStartTimeout int
	IdleTimeoutMinutes  int
	SessionName         string

	// Auth
	EnableTokenAuth bool
	AuthToken       string

	// SSL
	EnableSSL   bool
	SSLCertFile string
	SSLKeyFile  string
	TLSConfig   *tls.Config

	// Logging
	LogLevel         string
	LogFile          string
	EnableWebLogging bool

	// Custom HTTP headers
	CustomHTTPHeaders map[string]string

	// Licensing
	UseExistingLicense bool
	NLMConnStr         string
	LicModeOverride    string

	// MATLAB
	MATLABStartupScript string
	EnableSimulink      bool
	ProfileStartup      bool

	// Attach mode — connect to existing MATLAB EC instead of spawning
	AttachECPort  int
	AttachMWAPIKey string

	// Computed paths
	MATLABCommand string
	DataDir       string // ~/.matlab/MWI
}

// IsAttachMode returns true when the proxy should connect to an existing
// MATLAB Embedded Connector rather than spawning a new MATLAB process.
func (c *Config) IsAttachMode() bool {
	return c.AttachECPort > 0 && c.AttachMWAPIKey != ""
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:                GetEnv(EnvAppHost, DefaultHost),
		Port:                GetEnvInt(EnvAppPort, 0),
		BaseURL:             normalizeBaseURL(GetEnv(EnvBaseURL, "/")),
		ProcessStartTimeout: GetEnvInt(EnvProcessStartTimeout, DefaultProcessStartTimeout),
		IdleTimeoutMinutes:  GetEnvInt(EnvIdleTimeout, 0),
		SessionName:         GetEnv(EnvSessionName, ""),
		EnableTokenAuth:     GetEnvBool(EnvEnableTokenAuth, true),
		AuthToken:           GetEnv(EnvAuthToken, ""),
		EnableSSL:           GetEnvBool(EnvEnableSSL, false),
		SSLCertFile:         GetEnv(EnvSSLCertFile, ""),
		SSLKeyFile:          GetEnv(EnvSSLKeyFile, ""),
		LogLevel:            GetEnv(EnvLogLevel, "INFO"),
		LogFile:             GetEnv(EnvLogFile, ""),
		EnableWebLogging:    GetEnvBool(EnvEnableWebLogging, false),
		UseExistingLicense:  GetEnvBool(EnvUseExistingLicense, false),
		NLMConnStr:          GetEnv(EnvMLMLicenseFile, ""),
		LicModeOverride:     GetEnv(EnvLicModeOverride, ""),
		MATLABStartupScript: GetEnv(EnvMATLABStartupScript, ""),
		EnableSimulink:      GetEnvBool(EnvEnableSimulink, false),
		ProfileStartup:      GetEnvBool(EnvProfileStartup, false),
		AttachECPort:        GetEnvInt(EnvAttachECPort, 0),
		AttachMWAPIKey:      GetEnv(EnvAttachMWAPIKey, ""),
	}

	if err := cfg.resolveCustomHTTPHeaders(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvCustomHTTPHeaders, err)
	}

	if err := cfg.resolveMATLAB(); err != nil {
		return nil, err
	}

	if err := cfg.resolveDataDir(); err != nil {
		return nil, err
	}

	if cfg.Port == 0 {
		port, err := findFreePort()
		if err != nil {
			return nil, fmt.Errorf("finding free port: %w", err)
		}
		cfg.Port = port
	}

	if cfg.EnableSSL {
		if err := cfg.resolveSSL(); err != nil {
			return nil, fmt.Errorf("configuring SSL: %w", err)
		}
	}

	if cfg.SessionName == "" && cfg.MATLABVersion != "" {
		cfg.SessionName = "MATLAB " + cfg.MATLABVersion
	}

	return cfg, nil
}

func (c *Config) ServerURL() string {
	scheme := "http"
	if c.EnableSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://localhost:%d%s", scheme, c.Port, c.BaseURL)
}

func (c *Config) LogsDir() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}
	return filepath.Join(c.DataDir, "hosts", hostname, "ports", fmt.Sprintf("%d", c.Port))
}

func (c *Config) resolveCustomHTTPHeaders() error {
	raw := GetEnv(EnvCustomHTTPHeaders, "")
	if raw == "" {
		c.CustomHTTPHeaders = make(map[string]string)
		return nil
	}
	headers := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return err
	}
	c.CustomHTTPHeaders = headers
	return nil
}

func (c *Config) resolveMATLAB() error {
	root := GetEnv(EnvCustomMATLABRoot, "")
	if root != "" {
		c.MATLABRoot = root
	} else {
		matlabPath, err := exec.LookPath("matlab")
		if err != nil {
			// MATLAB not found - will need licensing/install before starting
			return nil
		}
		// Resolve symlinks
		resolved, err := filepath.EvalSymlinks(matlabPath)
		if err != nil {
			resolved = matlabPath
		}
		// matlab binary is typically at <root>/bin/matlab
		c.MATLABRoot = filepath.Dir(filepath.Dir(resolved))
	}

	if c.MATLABRoot != "" {
		c.MATLABVersion = detectMATLABVersion(c.MATLABRoot)
		if runtime.GOOS == "windows" {
			c.MATLABCommand = filepath.Join(c.MATLABRoot, "bin", "matlab.exe")
		} else {
			c.MATLABCommand = filepath.Join(c.MATLABRoot, "bin", "matlab")
		}
	}

	return nil
}

func detectMATLABVersion(root string) string {
	versionFile := filepath.Join(root, "VersionInfo.xml")
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	content := string(data)
	// Simple extraction of <release>R2024a</release>
	start := strings.Index(content, "<release>")
	if start == -1 {
		return ""
	}
	start += len("<release>")
	end := strings.Index(content[start:], "</release>")
	if end == -1 {
		return ""
	}
	return content[start : start+end]
}

func (c *Config) resolveDataDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	c.DataDir = filepath.Join(home, ".matlab", "MWI")
	return os.MkdirAll(c.DataDir, 0700)
}

func (c *Config) resolveSSL() error {
	if c.SSLCertFile != "" && c.SSLKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.SSLCertFile, c.SSLKeyFile)
		if err != nil {
			return fmt.Errorf("loading SSL cert/key: %w", err)
		}
		c.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		return nil
	}
	// Generate self-signed certificate
	cert, err := generateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("generating self-signed cert: %w", err)
	}
	c.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	return nil
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"MATLAB Proxy"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func normalizeBaseURL(base string) string {
	if base == "" {
		return "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return "/"
	}
	return base
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
