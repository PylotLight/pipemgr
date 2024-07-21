package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v2"
)

type Config struct {
	PAT       string   `yaml:"pat"`
	Org       string   `yaml:"org"`
	Project   string   `yaml:"project"`
	Pipelines []string `yaml:"pipelines"`
}

func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	return &config, nil
}

func ExtractBuildID(url string) (int, error) {
	re := regexp.MustCompile(`buildId=(\d+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return 0, fmt.Errorf("buildId not found in URL")
	}
	return strconv.Atoi(matches[1])
}
