package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host            string   `yaml:"host"`
	Port            int      `yaml:"port"`
	Timezone        int      `yaml:"timezone"`
	BannedUsers     []string `yaml:"banned_users"`
	NotifierWebhook string   `yaml:"notifier_webhook"`
}

var globalConfig = Config{
	Host:            "0.0.0.0",
	Port:            9812,
	Timezone:        7,
	BannedUsers:     []string{},
	NotifierWebhook: "",
}

func LoadConfig(path string) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Parse yaml
	var temp Config
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Apply values if present
	if temp.Host != "" {
		globalConfig.Host = temp.Host
	}
	if temp.Port != 0 {
		globalConfig.Port = temp.Port
	}
	globalConfig.Timezone = temp.Timezone
	if temp.BannedUsers != nil {
		globalConfig.BannedUsers = temp.BannedUsers
	}
	if temp.NotifierWebhook != "" {
		globalConfig.NotifierWebhook = temp.NotifierWebhook
	}

	return nil
}
