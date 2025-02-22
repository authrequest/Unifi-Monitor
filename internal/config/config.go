package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	DiscordWebhookURL string `yaml:"discord_webhook_url"`
	SaveBatchSize     int    `yaml:"save_batch_size"`
	HomeURL           string `yaml:"home_url"`
	ProductsFile      string `yaml:"products_file"`
}

func Load() (*Config, error) {
	cfg := &Config{
		SaveBatchSize: 2,
		HomeURL:       "https://store.ui.com/us/en",
		ProductsFile:  "products.json",
	}

	// Try environment variables first
	if url := os.Getenv("DISCORD_WEBHOOK_URL"); url != "" {
		cfg.DiscordWebhookURL = url
		return cfg, nil
	}

	// Try config file
	data, err := os.ReadFile("./config.yml")
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
