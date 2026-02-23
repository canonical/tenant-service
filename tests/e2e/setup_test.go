// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	hydra "github.com/ory/hydra-client-go/v2"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
)

const (
	defaultBaseURL = "http://localhost:8000"
	fgaAPIToken    = "42" // Matches docker-compose
)

var (
	testEnv      *TestEnvironment
	clientId     string
	clientSecret string
)

type TestEnvironment struct {
	Compose    tc.ComposeStack
	Cmd        *exec.Cmd
	BaseURL    string
	CancelFunc context.CancelFunc
	BinPath    string
}

func TestMain(m *testing.M) {
	var err error
	// Check if we should use existing deployment
	if os.Getenv("E2E_USE_EXISTING_DEPLOYMENT") == "true" {
		fmt.Println("Using existing deployment...")
		os.Exit(m.Run())
	}

	fmt.Println("Starting test environment...")
	testEnv, err = setupTestEnvironment()
	if err != nil {
		fmt.Printf("Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup explicitly before exit (defer won't run with os.Exit)
	if testEnv != nil {
		testEnv.Teardown()
	}

	os.Exit(code)
}

func setupTestEnvironment() (*TestEnvironment, error) {
	var (
		compose tc.ComposeStack
		binPath string
	)

	ctx, cancel := context.WithCancel(context.Background())

	cleanup := func() {
		if compose != nil {
			compose.Down(context.Background(), tc.RemoveOrphans(true), tc.RemoveImagesLocal)
		}
		if binPath != "" {
			os.Remove(binPath)
		}
		cancel()
	}

	// Locate docker-compose file
	rootDir, err := findRootDir()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to find root dir: %w", err)
	}
	composeFile := filepath.Join(rootDir, "docker-compose.dev.yml")

	// Build App
	binPath, err = buildApp(rootDir)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to build app: %w", err)
	}

	// Start Docker Compose
	// We need to use valid identifier for the project name
	identifier := fmt.Sprintf("tenant-e2e-%d", time.Now().Unix())
	compose, err = tc.NewDockerCompose(composeFile)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create docker compose: %w", err)
	}

	// Set project name to avoid conflicts and suppress docker-compose warnings
	compose = compose.WithEnv(map[string]string{
		"COMPOSE_PROJECT_NAME": identifier,
		"CLIENT_ID":            "", // Suppress warning - not used in E2E tests
		"CLIENT_SECRET":        "", // Suppress warning - not used in E2E tests
	})

	// Start services
	err = compose.Up(ctx, tc.Wait(false))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to start docker compose: %w", err)
	}

	// Wait for OpenFGA
	openfgaURL := "http://localhost:8080"
	if err := waitForHTTP(ctx, openfgaURL+"/healthz"); err != nil {
		cleanup()
		return nil, fmt.Errorf("openfga not ready: %w", err)
	}

	// Run Migrations
	dsn := "postgres://tenants:tenants@localhost:5432/tenants?sslmode=disable"
	if err := runMigrations(ctx, binPath, dsn); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Setup OpenFGA Model
	storeID, modelID, err := setupOpenFGA(ctx, binPath, openfgaURL)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to setup openfga: %w", err)
	}

	// Setup Hydra OAuth2 client for authentication
	clientId, clientSecret, err = setupHydraClient(ctx, "E2E Test Client")
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to setup hydra client: %w", err)
	}

	// Start the service with authentication enabled
	envVars := map[string]string{
		"DSN":                             dsn,
		"KRATOS_ADMIN_URL":                "http://localhost:4434",
		"OPENFGA_API_SCHEME":              "http",
		"OPENFGA_API_HOST":                "localhost:8080",
		"OPENFGA_STORE_ID":                storeID,
		"OPENFGA_AUTHORIZATION_MODEL_ID":  modelID,
		"OPENFGA_API_TOKEN":               fgaAPIToken,
		"PORT":                            "8000",
		"LOG_LEVEL":                       "debug",
		"TRACING_ENABLED":                 "false",
		"MONITORING_ENABLED":              "false",
		"AUTHENTICATION_ENABLED":          "true",
		"AUTHENTICATION_ISSUER":           "http://localhost:4444",
		"AUTHENTICATION_ALLOWED_SUBJECTS": clientId,
	}

	cmd, err := startServer(ctx, binPath, envVars)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	// Wait for Server
	baseURL := "http://localhost:8000"
	if err := waitForHTTP(ctx, "http://localhost:8000/api/v0/status"); err != nil {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cleanup()
		return nil, fmt.Errorf("server not ready: %w", err)
	}

	return &TestEnvironment{
		Compose:    compose,
		Cmd:        cmd,
		BaseURL:    baseURL,
		CancelFunc: cancel,
		BinPath:    binPath,
	}, nil
}

func (e *TestEnvironment) Teardown() {
	fmt.Println("Tearing down test environment...")
	if e.Cmd != nil && e.Cmd.Process != nil {
		fmt.Println("Stopping service process...")
		e.Cmd.Process.Kill()
		// Give the process a moment to flush I/O before waiting
		time.Sleep(100 * time.Millisecond)
		e.Cmd.Wait()
	}
	if e.BinPath != "" {
		os.Remove(e.BinPath)
	}
	if e.Compose != nil {
		// Stop and remove all containers, networks, and volumes
		fmt.Println("Stopping Docker Compose stack...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.Compose.Down(ctx, tc.RemoveOrphans(true), tc.RemoveImagesLocal, tc.RemoveVolumes(true)); err != nil {
			fmt.Printf("Warning: failed to cleanly stop compose: %v\n", err)
		}
	}
	if e.CancelFunc != nil {
		e.CancelFunc()
	}
	fmt.Println("Teardown complete")
}

func findRootDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.dev.yml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("root dir not found")
		}
		dir = parent
	}
}

func buildApp(rootDir string) (string, error) {
	binPath := filepath.Join(os.TempDir(), fmt.Sprintf("tenant-service-e2e-%d", time.Now().UnixNano()))
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = rootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return binPath, nil
}

func startServer(ctx context.Context, binPath string, envVars map[string]string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binPath, "serve")
	cmd.Env = os.Environ()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Use pipes instead of directly sharing os.Stdout/Stderr to avoid I/O incomplete issues
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Copy output in background goroutines
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	return cmd, nil
}

func waitForHTTP(ctx context.Context, url string) error {
	// Allow override from environment for CI
	timeoutDuration := 30 * time.Second
	if envTimeout := os.Getenv("E2E_STARTUP_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil {
			timeoutDuration = d
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(timeoutDuration)
	client := &http.Client{Timeout: 1 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for %s", url)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
		}
	}
}

func runMigrations(ctx context.Context, binPath, dsn string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for migrations")
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, binPath, "migrate", "up", "--dsn", dsn)
			_, err := cmd.CombinedOutput()
			if err == nil {
				return nil
			}
		}
	}
}

func setupOpenFGA(ctx context.Context, binPath, apiURL string) (string, string, error) {
	cmd := exec.CommandContext(ctx, binPath, "create-fga-model",
		"--fga-api-url", apiURL,
		"--fga-api-token", fgaAPIToken,
		"--format", "json",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("failed to create fga model: %v, output: %s", err, string(output))
	}

	var result struct {
		StoreID string `json:"store_id"`
		ModelID string `json:"model_id"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return "", "", fmt.Errorf("failed to parse fga model output: %v, output: %s", err, string(output))
	}

	return result.StoreID, result.ModelID, nil
}

func setupHydraClient(ctx context.Context, clientName string) (string, string, error) {
	// Wait for Hydra to be ready
	hydraAdminURL := "http://localhost:4445"
	if err := waitForHTTP(ctx, hydraAdminURL+"/health/ready"); err != nil {
		return "", "", fmt.Errorf("hydra not ready: %w", err)
	}

	// Create Hydra admin client using SDK
	configuration := hydra.NewConfiguration()
	configuration.Servers = []hydra.ServerConfiguration{
		{
			URL: hydraAdminURL,
		},
	}
	apiClient := hydra.NewAPIClient(configuration)

	// Create OAuth2 client for client credentials flow
	grantTypes := []string{"client_credentials"}

	client := hydra.NewOAuth2Client()
	client.SetClientName(clientName)
	client.SetGrantTypes(grantTypes)

	createdClient, _, err := apiClient.OAuth2API.CreateOAuth2Client(ctx).OAuth2Client(*client).Execute()
	if err != nil {
		return "", "", fmt.Errorf("failed to create hydra client via SDK: %w", err)
	}

	if createdClient.ClientId == nil || createdClient.ClientSecret == nil {
		return "", "", fmt.Errorf("hydra client creation succeeded but missing credentials")
	}

	return *createdClient.ClientId, *createdClient.ClientSecret, nil
}

func getJWTToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	token, _, err := getJWTTokenWithExpiry(ctx, clientID, clientSecret)
	return token, err
}

func getJWTTokenWithExpiry(ctx context.Context, clientID, clientSecret string) (string, int, error) {
	// Get token from Hydra using client credentials flow
	tokenURL := "http://localhost:4444/oauth2/token"
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create token request: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get JWT token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", 0, fmt.Errorf("token request failed with status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return "", 0, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, fmt.Errorf("failed to parse token response: %v", err)
	}

	return result.AccessToken, result.ExpiresIn, nil
}
