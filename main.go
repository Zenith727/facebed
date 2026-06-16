package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	configFile string
)

func parseCommandLine() {
	flag.StringVar(&configFile, "c", "", "config yaml file path")
	flag.StringVar(&configFile, "config", "", "config yaml file path")
	flag.Parse()
}

func main() {
	parseCommandLine()

	// Load configuration
	if configFile != "" {
		if err := LoadConfig(configFile); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
	}

	// Load cookies
	globalCookieStore.Load()

	// Initialize crawler regex patterns
	InitCrawlerDetector()

	// Setup Server
	mux := http.NewServeMux()

	// Favicon and Banner routes
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		http.ServeFile(w, r, "./assets/favicon.ico")
	})

	mux.HandleFunc("/banner.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, "./assets/banner.png")
	})

	// Root route
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		indexBytes, err := os.ReadFile("assets/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		htmlStr := strings.ReplaceAll(string(indexBytes), "{|CREDIT|}", GetCredit())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlStr))
	})

	// Fallback path router for Facebook URLs
	mux.HandleFunc("/", handleEmbedRouter)

	addr := fmt.Sprintf("%s:%d", globalConfig.Host, globalConfig.Port)
	log.Printf("listening on http://%s", addr)
	if err := http.ListenAndServe(addr, logRequest(mux)); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Basic logging in line with python wraps
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.String())
		next.ServeHTTP(w, r)
	})
}

func handleEmbedRouter(w http.ResponseWriter, r *http.Request) {
	// Reconstruct path + query params if present, similar to python
	path := strings.TrimPrefix(r.URL.Path, "/")
	if r.URL.RawQuery != "" {
		path = path + "?" + r.URL.RawQuery
	}

	// 1. Check if the request is for /text version
	textOnly := false
	if strings.HasSuffix(path, "/text") {
		textOnly = true
		path = strings.TrimSuffix(path, "/text")
	}

	// 2. Handle /dump diagnostics route
	dumpRegex := regexp.MustCompile(`(?i)^(.*)/dump/?$`)
	if dumpRegex.MatchString(path) {
		matches := dumpRegex.FindStringSubmatch(path)
		dumpPath := matches[1]
		log.Printf("Dump report requested for: /%s", dumpPath)

		go func(dp string) {
			urlStr := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(dp, "/"))
			client := GetStealthClient()
			reqR := client.R()
			for k, v := range GetScrapeHeaders() {
				reqR.SetHeader(k, v)
			}
			for k, v := range globalCookieStore.GetMap() {
				reqR.SetCookies(&http.Cookie{Name: k, Value: v})
			}
			resp, err := reqR.Get(urlStr)
			if err == nil {
				defer resp.Body.Close()
				htmlBytes, _ := io.ReadAll(resp.Body)
				htmlStr := string(htmlBytes)

				filename := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(dp, "_")
				if len(filename) > 80 {
					filename = filename[:80]
				}
				filename += "_dump.html"

				embed := &DiscordEmbed{
					Title:       "manual dump report",
					Description: fmt.Sprintf("🔗 [`/%s`](%s)\n📋 Manual dump requested by user", dp, urlStr),
					Color:       0x3498DB,
					Fields: []DiscordEmbedField{
						{Name: "Attached Payload", Value: fmt.Sprintf("`%s`", filename), Inline: true},
						{Name: "Response Size", Value: fmt.Sprintf("%d chars", len(htmlStr)), Inline: true},
					},
				}
				WarnWebhook("", htmlBytes, filename, embed)
			}
		}(dumpPath)

		// Continue parsing normally using stripped path
		path = dumpPath
	}

	// 3. Image in comment (type=3 query param)
	isType3Comment := false
	if r.URL.Query().Get("type") == "3" {
		isType3Comment = true
	}

	// 4. Redirect non-crawlers back to facebook
	if !IsCrawler(r.UserAgent()) {
		fbRedirectURL := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(path, "/"))
		w.Header().Set("Location", fbRedirectURL)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusMovedPermanently)
		_, _ = w.Write([]byte(FormatRedirectPage(fbRedirectURL)))
		return
	}

	// Compile parsing response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var embedHTML string
	var err error

	if isType3Comment {
		var p ParsedPost
		parser := PhotocomParser{}
		p, err = parser.ProcessPost(path)
		if err == nil {
			embedHTML = FormatFullPostEmbed(p, textOnly)
		}
	} else {
		embedHTML, err = routeEmbed(path, textOnly)
	}

	if err != nil {
		log.Printf("Error processing /%s: %v", path, err)
		fbURL := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(path, "/"))

		// Check if it's a parser bug we should report via webhook
		if fbErr, ok := err.(*FacebedError); ok {
			// Report unexpected parser crash/parsing failures to notifier webhook
			if fbErr.HTML != "" {
				filename := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(path, "_")
				if len(filename) > 80 {
					filename = filename[:80]
				}
				filename += ".html"

				embed := &DiscordEmbed{
					Title:       "embed failure",
					Description: fmt.Sprintf("🔗 [`/%s`](%s)\n🚩 %s and attached file", path, fbErr.URL, fbErr.Message),
					Color:       0xFF0000,
					Fields: []DiscordEmbedField{
						{Name: "Attached Payload", Value: fmt.Sprintf("`%s`", filename), Inline: true},
					},
				}
				WarnWebhook("", []byte(fbErr.HTML), filename, embed)
			} else {
				embed := &DiscordEmbed{
					Title:       "embed failure",
					Description: fmt.Sprintf("🔗 [`/%s`](%s)\n🚩 %s", path, fbErr.URL, fbErr.Message),
					Color:       0xFF0000,
				}
				WarnWebhook("", nil, "", embed)
			}
		}

		_, _ = w.Write([]byte(FormatErrorMessageEmbed(fbURL)))
		return
	}

	_, _ = w.Write([]byte(embedHTML))
}

func routeEmbed(path string, textOnly bool) (string, error) {
	// Share links
	if regexp.MustCompile(`(?i)^/?share/v/.*`).MatchString(path) || regexp.MustCompile(`(?i)^/?share/([pr]/)?[a-zA-Z0-9-._]*(/)?`).MatchString(path) {
		resolved := ResolveShareLink(path)
		if resolved == "" {
			return FormatErrorMessageEmbed(fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(path, "/"))), nil
		}
		path = resolved
	}

	// Videos path replacement to reel
	videoRegex := regexp.MustCompile(`(?i)/videos/(\d+).*`)
	if videoRegex.MatchString(path) {
		matches := videoRegex.FindStringSubmatch(path)
		path = fmt.Sprintf("reel/%s", matches[1])
	}

	// Reels
	if regexp.MustCompile(`(?i)^/?reel/[0-9]+`).MatchString(path) {
		parser := ReelsParser{}
		post, err := parser.ProcessPost(path)
		if err != nil {
			return "", err
		}
		return FormatReelPostEmbed(post, textOnly), nil
	}

	// Single Photo
	parsedURL, parseErr := url.Parse("https://www.facebook.com/" + strings.TrimPrefix(path, "/"))
	if parseErr == nil {
		photoPath := strings.TrimPrefix(parsedURL.Path, "/")
		if regexp.MustCompile(`(?i)^/?photo(\.php)?/?$`).MatchString(photoPath) {
			parser := SinglePhotoParser{}
			post, err := parser.ProcessPost(path)
			if err != nil {
				return "", err
			}
			return FormatFullPostEmbed(post, textOnly), nil
		}
		if regexp.MustCompile(`(?i)^/?watch/?$`).MatchString(photoPath) {
			parser := VideoWatchParser{}
			post, err := parser.ProcessPost(path)
			if err != nil {
				return "", err
			}
			return FormatReelPostEmbed(post, textOnly), nil
		}
	}

	// Standard Post
	if isFacebookURL(path) {
		post, err := ProcessPost(path)
		if err != nil {
			return "", err
		}
		return FormatFullPostEmbed(post, textOnly), nil
	}

	// Fallback to project info
	return FormatErrorMessageEmbed("https://git.facebed.com"), nil
}

func isFacebookURL(path string) bool {
	u, err := url.Parse("https://www.facebook.com/" + strings.TrimPrefix(path, "/"))
	if err != nil {
		return false
	}

	p := u.Path
	usernamePattern := `[a-zA-Z0-9-._]*`

	isGroupPost := regexp.MustCompile(fmt.Sprintf(`^/groups/%s`, usernamePattern)).MatchString(p)
	isPermalink := strings.HasPrefix(p, "/permalink.php")
	isStory := strings.HasPrefix(p, "/story.php")
	isPost := regexp.MustCompile(fmt.Sprintf(`^/%s/posts`, usernamePattern)).MatchString(p)
	isPhoto := strings.HasPrefix(p, "/photo")

	return isPermalink || isPost || isStory || isPhoto || isGroupPost
}
