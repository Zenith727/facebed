package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type CookieEntry struct {
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	ExpirationDate float64 `json:"expirationDate"`
}

type CookieStore struct {
	mu      sync.RWMutex
	cookies []CookieEntry
	fn      string
}

var globalCookieStore = &CookieStore{
	fn: "cookies.json",
}

func (cs *CookieStore) Load() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, err := os.Stat(cs.fn); os.IsNotExist(err) {
		log.Println("cookies.json not found, non incognito-viewable posts will NOT work")
		return
	}

	data, err := os.ReadFile(cs.fn)
	if err != nil {
		log.Printf("failed to read cookies file: %v", err)
		return
	}

	var parsed []CookieEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Printf("failed to parse cookies.json: %v", err)
		return
	}

	cs.cookies = parsed
	log.Printf("loaded %d cookies from %s", len(cs.cookies), cs.fn)

	// Validate expiration of loaded cookies
	cs.validate()
}

func (cs *CookieStore) validate() {
	now := float64(time.Now().Unix())
	hasExpired := false
	for _, cookie := range cs.cookies {
		exp := cookie.ExpirationDate
		if exp == 0 {
			exp = 2147483647 // 2**31 - 1 (default to far future if missing)
		}
		if exp <= now {
			hasExpired = true
			break
		}
	}

	if hasExpired {
		log.Println("Warning: cookies.json contains expired cookies!")
		WarnWebhook("@everyone cookies expired", nil, "", nil)
	}
}

func (cs *CookieStore) GetMap() map[string]string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	now := float64(time.Now().Unix())
	res := make(map[string]string)
	for _, cookie := range cs.cookies {
		exp := cookie.ExpirationDate
		if exp == 0 {
			exp = 2147483647
		}
		if exp > now {
			res[cookie.Name] = cookie.Value
		}
	}
	return res
}
