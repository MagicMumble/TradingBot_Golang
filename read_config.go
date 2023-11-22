package main

import (
	"flag"
	"gopkg.in/yaml.v3"
	"log"
	"os"
)

type Config struct {
	Token     string `yaml:"token"`
	Port      int    `yaml:"server_port"`
	TargetAPI string `yaml:"target_api"`
	AccountID string `yaml:"account_id"`
}

func getConfigFilePath() string {
	configFilePath := flag.String("config", "", "a filepath to the config file")
	flag.Parse()
	return *configFilePath
}

func readConfig(path string) Config {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Cannot read config file: %v", err)
	}
	obj := make(map[string]interface{})
	err = yaml.Unmarshal(yamlFile, obj)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return Config{
		Token:     obj["token"].(string),
		Port:      obj["server_port"].(int),
		TargetAPI: obj["target_api"].(string),
		AccountID: obj["account_id"].(string),
	}
}

func getConfigParams() Config {
	configFilePath := getConfigFilePath()
	config := readConfig(configFilePath)
	return config
}
