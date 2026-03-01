package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
)

var (
	serverURL string
	apiKey    string
)

func globalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "server", Aliases: []string{"s"}, EnvVars: []string{"MODELGATE_SERVER"}},
		&cli.StringFlag{Name: "api-key", Aliases: []string{"k"}, EnvVars: []string{"MODELGATE_API_KEY"}},
	}
}

func joinFlags(a, b []cli.Flag) []cli.Flag {
	result := make([]cli.Flag, len(a)+len(b))
	copy(result, a)
	copy(result[len(a):], b)
	return result
}

func getGlobalFlags(c *cli.Context) (string, string) {
	s := c.String("server")
	k := c.String("api-key")
	if s == "" {
		s = os.Getenv("MODELGATE_SERVER")
	}
	if k == "" {
		k = os.Getenv("MODELGATE_API_KEY")
	}
	if s == "" {
		s = "http://localhost:8080"
	}
	return s, k
}

func main() {
	app := &cli.App{
		Name:  "modelgate-cli",
		Usage: "ModelGate Admin CLI",
		Commands: []*cli.Command{
			{
				Name:  "key",
				Usage: "Manage API Keys",
				Subcommands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List all API keys",
						Flags:  globalFlags(),
						Action: listKeys,
					},
					{
						Name:  "create",
						Usage: "Create a new API key",
						Flags: joinFlags(globalFlags(), []cli.Flag{
							&cli.StringFlag{Name: "name", Aliases: []string{"n"}, Required: true},
							&cli.Int64Flag{Name: "quota", Aliases: []string{"q"}, Value: 1000000},
							&cli.IntFlag{Name: "rate-limit", Aliases: []string{"r"}, Value: 60},
							&cli.StringFlag{Name: "tier", Aliases: []string{"t"}, Value: "free"},
							&cli.StringFlag{Name: "default-model", Aliases: []string{"d"}},
							&cli.StringFlag{Name: "allowed-ips", Aliases: []string{"i"}},
						}),
						Action: createKey,
					},
					{
						Name:   "delete",
						Usage:  "Delete an API key",
						Flags:  globalFlags(),
						Args:   true,
						Action: deleteKey,
					},
					{
						Name:  "update",
						Usage: "Update an API key",
						Flags: joinFlags(globalFlags(), []cli.Flag{
							&cli.Int64Flag{Name: "quota", Aliases: []string{"q"}, Value: 0},
							&cli.IntFlag{Name: "rate-limit", Aliases: []string{"r"}, Value: 0},
							&cli.StringFlag{Name: "status", Value: ""},
							&cli.StringFlag{Name: "tier", Value: ""},
							&cli.StringFlag{Name: "default-model", Value: ""},
						}),
						Action: updateKey,
					},
				},
			},
			{
				Name:  "model",
				Usage: "Manage Models",
				Subcommands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List all models",
						Flags:  globalFlags(),
						Action: listModels,
					},
					{
						Name:  "create",
						Usage: "Create a new model",
						Flags: joinFlags(globalFlags(), []cli.Flag{
							&cli.StringFlag{Name: "name", Aliases: []string{"n"}, Required: true},
							&cli.StringFlag{Name: "backend", Aliases: []string{"b"}, Required: true},
							&cli.StringFlag{Name: "base-url", Aliases: []string{"u"}},
						}),
						Action: createModel,
					},
					{
						Name:   "delete",
						Usage:  "Delete a model",
						Flags:  globalFlags(),
						Args:   true,
						Action: deleteModel,
					},
					{
						Name:  "update",
						Usage: "Update a model",
						Flags: joinFlags(globalFlags(), []cli.Flag{
							&cli.StringFlag{Name: "name", Aliases: []string{"n"}},
							&cli.StringFlag{Name: "backend", Aliases: []string{"b"}},
							&cli.StringFlag{Name: "base-url", Aliases: []string{"u"}},
							&cli.BoolFlag{Name: "enabled", Aliases: []string{"e"}},
						}),
						Action: updateModel,
					},
				},
			},
			{
				Name:  "sync",
				Usage: "Sync models from upstream backends",
				Flags: joinFlags(globalFlags(), []cli.Flag{
					&cli.StringFlag{Name: "config", Aliases: []string{"c"}, DefaultText: "./configs/config.yaml"},
					&cli.BoolFlag{Name: "dry-run", Aliases: []string{"d"}},
				}),
				Action: syncModels,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getClient() *http.Client {
	return &http.Client{}
}

func makeRequest(method, path string, body io.Reader) ([]byte, error) {
	return makeRequestWithAuth(method, path, body, serverURL, apiKey)
}

func makeRequestWithAuth(method, path string, body io.Reader, srv, key string) ([]byte, error) {
	url := strings.TrimRight(srv, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}

	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := getClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w (server: %s)", err, url)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func listKeys(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	if apiKey == "" {
		return fmt.Errorf("API key is required. Use --api-key or -k flag")
	}

	data, err := makeRequest("GET", "/admin/keys", nil)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	var keys []map[string]interface{}
	if err := json.Unmarshal(data, &keys); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No API keys found.")
		return nil
	}

	fmt.Println("API Keys:")
	fmt.Println("ID\tName\t\tKey\t\t\t\t\tQuota\t\tUsed\t\tRate Limit\tStatus")
	for _, k := range keys {
		fmt.Printf("%v\t%s\t\t%v\t\t%v\t\t%v\t\t%v\t\t%s\n",
			k["id"], k["name"], k["key"], k["quota"], k["quota_used"], k["rate_limit"], k["status"])
	}
	return nil
}

func createKey(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	if apiKey == "" {
		return fmt.Errorf("API key is required. Use --api-key or -k flag")
	}

	tier := c.String("tier")
	defaultModel := c.String("default-model")

	payload := fmt.Sprintf(`{
		"name": "%s",
		"quota": %d,
		"rate_limit": %d,
		"tier": "%s",
		"default_model": "%s",
		"allowed_ips": "%s"
	}`, c.String("name"), c.Int64("quota"), c.Int("rate-limit"), tier, defaultModel, c.String("allowed-ips"))

	data, err := makeRequest("POST", "/admin/keys", strings.NewReader(payload))
	if err != nil {
		return err
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	fmt.Println("API Key created successfully!")
	fmt.Printf("Key: %s\n", result["key"])
	fmt.Printf("Name: %s\n", result["name"])
	fmt.Printf("Tier: %s\n", result["tier"])
	fmt.Printf("Default Model: %s\n", result["default_model"])
	fmt.Printf("Quota: %v\n", result["quota"])
	return nil
}

func deleteKey(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	id := c.Args().First()
	_, err := makeRequest("DELETE", "/admin/keys/"+id, nil)
	if err != nil {
		return err
	}

	fmt.Println("API Key deleted successfully!")
	return nil
}

func updateKey(c *cli.Context) error {
	srv, key := getGlobalFlags(c)

	id := c.Args().First()
	quota := c.Int64("quota")
	rateLimit := c.Int("rate-limit")
	status := c.String("status")
	tier := c.String("tier")
	defaultModel := c.String("default-model")

	updates := []string{}
	if quota > 0 {
		updates = append(updates, fmt.Sprintf(`"quota": %d`, quota))
	}
	if rateLimit > 0 {
		updates = append(updates, fmt.Sprintf(`"rate_limit": %d`, rateLimit))
	}
	if status != "" {
		updates = append(updates, fmt.Sprintf(`"status": "%s"`, status))
	}
	if tier != "" {
		updates = append(updates, fmt.Sprintf(`"tier": "%s"`, tier))
	}
	if defaultModel != "" {
		updates = append(updates, fmt.Sprintf(`"default_model": "%s"`, defaultModel))
	}

	payload := "{" + strings.Join(updates, ",") + "}"

	if len(updates) == 0 {
		return fmt.Errorf("no updates provided")
	}

	_, err := makeRequestWithAuth("PUT", "/admin/keys/"+id, strings.NewReader(payload), srv, key)
	if err != nil {
		return err
	}

	fmt.Println("API Key updated successfully!")
	return nil
}

func listModels(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	data, err := makeRequest("GET", "/admin/models", nil)
	if err != nil {
		return err
	}

	var models []map[string]interface{}
	json.Unmarshal(data, &models)

	fmt.Println("Models:")
	fmt.Println("ID\tName\t\tBackend\t\tBase URL\t\tEnabled")
	for _, m := range models {
		fmt.Printf("%v\t%s\t\t%s\t\t%s\t\t%v\n",
			m["id"], m["name"], m["backend_type"], m["base_url"], m["enabled"])
	}
	return nil
}

func createModel(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	payload := fmt.Sprintf(`{
		"name": "%s",
		"backend_type": "%s",
		"base_url": "%s",
		"api_key": "%s",
		"enabled": true
	}`, c.String("name"), c.String("backend"), c.String("base-url"), c.String("api-key"))

	_, err := makeRequest("POST", "/admin/models", strings.NewReader(payload))
	if err != nil {
		return err
	}

	fmt.Println("Model created successfully!")
	return nil
}

func deleteModel(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	id := c.Args().First()
	_, err := makeRequest("DELETE", "/admin/models/"+id, nil)
	if err != nil {
		return err
	}

	fmt.Println("Model deleted successfully!")
	return nil
}

func updateModel(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	id := c.Args().First()

	updates := []string{}
	if c.IsSet("name") {
		updates = append(updates, fmt.Sprintf(`"name": "%s"`, c.String("name")))
	}
	if c.IsSet("backend") {
		updates = append(updates, fmt.Sprintf(`"backend_type": "%s"`, c.String("backend")))
	}
	if c.IsSet("base-url") {
		updates = append(updates, fmt.Sprintf(`"base_url": "%s"`, c.String("base-url")))
	}
	if c.IsSet("enabled") {
		updates = append(updates, fmt.Sprintf(`"enabled": %t`, c.Bool("enabled")))
	}

	payload := "{" + strings.Join(updates, ",") + "}"

	_, err := makeRequest("PUT", "/admin/models/"+id, strings.NewReader(payload))
	if err != nil {
		return err
	}

	fmt.Println("Model updated successfully!")
	return nil
}

type AdapterConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

type AdaptersConfig struct {
	Ollama   AdapterConfig `mapstructure:"ollama"`
	VLLM     AdapterConfig `mapstructure:"vllm"`
	LlamaCPP AdapterConfig `mapstructure:"llamacpp"`
	OpenAI   AdapterConfig `mapstructure:"openai"`
	API3     AdapterConfig `mapstructure:"api3"`
}

func syncModels(c *cli.Context) error {
	serverURL = c.String("server")
	apiKey = c.String("api-key")

	if apiKey == "" {
		return fmt.Errorf("API key is required. Use --api-key or -k flag")
	}

	configPath := c.String("config")
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var adapters AdaptersConfig
	if err := viper.UnmarshalKey("adapters", &adapters); err != nil {
		return fmt.Errorf("failed to parse adapters config: %w", err)
	}

	dryRun := c.Bool("dry-run")

	type backend struct {
		name    string
		baseURL string
	}

	backends := []backend{
		{"ollama", adapters.Ollama.BaseURL},
		{"vllm", adapters.VLLM.BaseURL},
		{"llamacpp", adapters.LlamaCPP.BaseURL},
		{"openai", adapters.OpenAI.BaseURL},
		{"api3", adapters.API3.BaseURL},
	}

	client := &http.Client{Timeout: 10 * time.Second}
	allModels := []map[string]string{}

	for _, b := range backends {
		if b.baseURL == "" {
			continue
		}

		models, err := fetchBackendModels(client, b.name, b.baseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch models from %s: %v\n", b.name, err)
			continue
		}

		for _, m := range models {
			m["backend"] = b.name
			m["base_url"] = b.baseURL
			allModels = append(allModels, m)
		}
		fmt.Fprintf(os.Stderr, "Fetched %d models from %s\n", len(models), b.name)
	}

	if len(allModels) == 0 {
		fmt.Println("No models found from backends.")
		return nil
	}

	fmt.Printf("\nFound %d models total:\n", len(allModels))
	for _, m := range allModels {
		fmt.Printf("  - %s (%s)\n", m["name"], m["backend"])
	}

	if dryRun {
		fmt.Println("\nDry run - no models created.")
		return nil
	}

	created := 0
	for _, m := range allModels {
		payload := fmt.Sprintf(`{
			"name": "%s",
			"backend_type": "%s",
			"base_url": "%s",
			"enabled": true
		}`, m["name"], m["backend"], m["base_url"])

		_, err := makeRequest("POST", "/admin/models", strings.NewReader(payload))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create model %s: %v\n", m["name"], err)
			continue
		}
		created++
	}

	fmt.Printf("\nSuccessfully created %d models!\n", created)
	return nil
}

func fetchBackendModels(client *http.Client, backend, baseURL string) ([]map[string]string, error) {
	var parseFunc func([]byte) ([]map[string]string, error)

	switch backend {
	case "ollama":
		parseFunc = parseOllamaModels
	case "vllm", "openai", "llamacpp":
		parseFunc = parseOpenAIModels
	case "api3":
		parseFunc = parseOpenAIModels
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}

	url, err := backendModelsURL(backend, baseURL)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseFunc(data)
}

func backendModelsURL(backend, baseURL string) (string, error) {
	normalizedBackend := strings.ToLower(strings.TrimSpace(backend))
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		return "", fmt.Errorf("base url is empty")
	}

	switch normalizedBackend {
	case "ollama":
		return normalizedBaseURL + "/api/tags", nil
	case "vllm", "openai", "llamacpp", "api3":
		if strings.HasSuffix(normalizedBaseURL, "/v1") {
			return normalizedBaseURL + "/models", nil
		}
		return normalizedBaseURL + "/v1/models", nil
	default:
		return "", fmt.Errorf("unknown backend: %s", backend)
	}
}

func parseOllamaModels(data []byte) ([]map[string]string, error) {
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	models := make([]map[string]string, len(resp.Models))
	for i, m := range resp.Models {
		models[i] = map[string]string{"name": m.Name}
	}
	return models, nil
}

func parseOpenAIModels(data []byte) ([]map[string]string, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	models := make([]map[string]string, len(resp.Data))
	for i, m := range resp.Data {
		models[i] = map[string]string{"name": m.ID}
	}
	return models, nil
}
