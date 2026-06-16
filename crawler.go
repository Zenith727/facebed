package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"regexp"
)

//go:embed crawler-user-agents.json
var crawlerUserAgentsJSON []byte

type CrawlerPattern struct {
	Pattern string `json:"pattern"`
}

var compiledPatterns []*regexp.Regexp

func InitCrawlerDetector() {
	var list []CrawlerPattern
	if err := json.Unmarshal(crawlerUserAgentsJSON, &list); err != nil {
		log.Fatalf("failed to parse embedded crawler-user-agents.json: %v", err)
	}

	for _, cp := range list {
		// Compile case-insensitively
		rx, err := regexp.Compile("(?i)" + cp.Pattern)
		if err != nil {
			// Skip Go-incompatible regexes (e.g. lookarounds) to prevent crashes
			continue
		}
		compiledPatterns = append(compiledPatterns, rx)
	}
	log.Printf("compiled %d crawler user-agent patterns", len(compiledPatterns))
}

func IsCrawler(ua string) bool {
	if ua == "" {
		return false
	}
	for _, rx := range compiledPatterns {
		if rx.MatchString(ua) {
			return true
		}
	}
	return false
}
