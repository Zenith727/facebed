package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestConfigLoad(t *testing.T) {
	configData := `
host: 127.0.0.1
port: 1234
timezone: -5
banned_users:
  - "banned1"
  - "banned2"
notifier_webhook: "https://discord.com/api/webhooks/12345"
`
	tmpFile := "test_config_temp.yaml"
	if err := os.WriteFile(tmpFile, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile)

	if err := LoadConfig(tmpFile); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if globalConfig.Host != "127.0.0.1" {
		t.Errorf("expected Host 127.0.0.1, got %s", globalConfig.Host)
	}
	if globalConfig.Port != 1234 {
		t.Errorf("expected Port 1234, got %d", globalConfig.Port)
	}
	if globalConfig.Timezone != -5 {
		t.Errorf("expected Timezone -5, got %d", globalConfig.Timezone)
	}
	if len(globalConfig.BannedUsers) != 2 || globalConfig.BannedUsers[0] != "banned1" {
		t.Errorf("banned users not parsed correctly: %v", globalConfig.BannedUsers)
	}
	if globalConfig.NotifierWebhook != "https://discord.com/api/webhooks/12345" {
		t.Errorf("unexpected Webhook: %s", globalConfig.NotifierWebhook)
	}
}

func TestCrawlerDetector(t *testing.T) {
	InitCrawlerDetector()

	crawlerUAs := []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_voiced.php)",
		"Twitterbot/1.0",
		"TelegramBot (like Twitterbot)",
		"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
	}

	browserUAs := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2.1 Safari/605.1.15",
		"",
	}

	for _, ua := range crawlerUAs {
		if !IsCrawler(ua) {
			t.Errorf("expected UA to be crawler: %q", ua)
		}
	}

	for _, ua := range browserUAs {
		if IsCrawler(ua) {
			t.Errorf("expected UA NOT to be crawler: %q", ua)
		}
	}
}

func TestJqHelpers(t *testing.T) {
	jsonStr := `
	{
		"data": {
			"node_v2": {
				"id": "123456",
				"wwwURL": "https://www.facebook.com/test",
				"feedback": {
					"i18n_reaction_count": "1.2K"
				}
			}
		}
	}`

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	id := JqFirst(parsed, "id")
	if id != "123456" {
		t.Errorf("expected id 123456, got %v", id)
	}

	reactCount := JqFirst(parsed, "i18n_reaction_count")
	if reactCount != "1.2K" {
		t.Errorf("expected reaction count 1.2K, got %v", reactCount)
	}

	if !JqHas(parsed, "wwwURL", "i18n_reaction_count") {
		t.Error("expected JqHas to return true")
	}

	if JqHas(parsed, "nonexistent_key") {
		t.Error("expected JqHas to return false")
	}
}

func TestIsFacebookURL(t *testing.T) {
	valid := []string{
		"groups/123456/posts/7890",
		"permalink.php?story_fbid=123",
		"story.php?story_fbid=456",
		"user/posts/101618037324",
		"photo.php?fbid=112233",
		"photo?fbid=445566",
	}

	invalid := []string{
		"some/other/url",
		"about",
		"favicon.ico",
	}

	for _, p := range valid {
		if !isFacebookURL(p) {
			t.Errorf("expected valid Facebook URL: %q", p)
		}
	}

	for _, p := range invalid {
		if isFacebookURL(p) {
			t.Errorf("expected invalid Facebook URL: %q", p)
		}
	}
}

func TestEmbedSystem(t *testing.T) {
	// Test FormatErrorMessageEmbed
	errEmbed := FormatErrorMessageEmbed("https://example.com/post?id=123")
	if !strings.Contains(errEmbed, "og:title") || !strings.Contains(errEmbed, "Log in or sign up to view") {
		t.Errorf("FormatErrorMessageEmbed missing og:title or content: %s", errEmbed)
	}
	if !strings.Contains(errEmbed, "https://example.com/post?id=123") {
		t.Errorf("FormatErrorMessageEmbed missing original URL: %s", errEmbed)
	}

	// Test FormatRedirectPage
	redirPage := FormatRedirectPage("https://example.com/redirect")
	if !strings.Contains(redirPage, `http-equiv="refresh"`) || !strings.Contains(redirPage, "https://example.com/redirect") {
		t.Errorf("FormatRedirectPage missing refresh or redirect URL: %s", redirPage)
	}

	// Setup Config for consistent Timezone and Credit
	globalConfig.Timezone = 7

	// Test FormatFullPostEmbed with images
	postWithImages := ParsedPost{
		AuthorName: "John Doe",
		Text:       "This is a post with images!",
		ImageLinks: []string{"https://example.com/img1.jpg", "https://example.com/img2.jpg"},
		URL:        "https://facebook.com/post123",
		Date:       1600000000,
		Likes:      "100",
		Comments:   "50",
		Shares:     "10",
	}

	htmlWithImages := FormatFullPostEmbed(postWithImages, false)
	if !strings.Contains(htmlWithImages, `content="John Doe"`) {
		t.Errorf("FormatFullPostEmbed missing author name: %s", htmlWithImages)
	}
	if !strings.Contains(htmlWithImages, `content="This is a post with images!"`) {
		t.Errorf("FormatFullPostEmbed missing post text: %s", htmlWithImages)
	}
	if !strings.Contains(htmlWithImages, `content="https://example.com/img1.jpg"`) || !strings.Contains(htmlWithImages, `content="https://example.com/img2.jpg"`) {
		t.Errorf("FormatFullPostEmbed missing image links: %s", htmlWithImages)
	}
	if !strings.Contains(htmlWithImages, "❤️ 100 • 💬 50 • 🔁 10") {
		t.Errorf("FormatFullPostEmbed missing or incorrect reactions: %s", htmlWithImages)
	}
	if !strings.Contains(htmlWithImages, "2020/09/13 19:26:40 UTC+07") {
		t.Errorf("FormatFullPostEmbed missing or incorrect timestamp: %s", htmlWithImages)
	}

	// Test FormatFullPostEmbed with textOnly = true (images should be stripped)
	htmlTextOnlyImages := FormatFullPostEmbed(postWithImages, true)
	if strings.Contains(htmlTextOnlyImages, `content="https://example.com/img1.jpg"`) {
		t.Errorf("FormatFullPostEmbed in textOnly mode should not contain image links: %s", htmlTextOnlyImages)
	}

	// Test FormatReelPostEmbed with video
	postWithVideo := ParsedPost{
		AuthorName: "Jane Doe",
		Text:       "This is a cool video!",
		VideoLinks: []string{"https://example.com/video.mp4"},
		URL:        "https://facebook.com/reel456",
		Date:       1600000000,
		Likes:      "1.5K",
		Comments:   "300",
		Shares:     "0",
	}

	htmlWithVideo := FormatFullPostEmbed(postWithVideo, false) // should route to FormatReelPostEmbed because VideoLinks is non-empty
	if !strings.Contains(htmlWithVideo, `content="Jane Doe"`) {
		t.Errorf("FormatReelPostEmbed missing author name: %s", htmlWithVideo)
	}
	if !strings.Contains(htmlWithVideo, `content="https://example.com/video.mp4"`) {
		t.Errorf("FormatReelPostEmbed missing video link: %s", htmlWithVideo)
	}
	// Likes formatted as "1.5K", shares should not show as they are 0
	if !strings.Contains(htmlWithVideo, "❤️ 1.5K • 💬 300") {
		t.Errorf("FormatReelPostEmbed missing or incorrect reactions: %s", htmlWithVideo)
	}

	// Test FormatReelPostEmbed with textOnly = true (videos should be stripped)
	htmlTextOnlyVideo := FormatFullPostEmbed(postWithVideo, true)
	if strings.Contains(htmlTextOnlyVideo, `content="https://example.com/video.mp4"`) {
		t.Errorf("FormatReelPostEmbed in textOnly mode should not contain video links: %s", htmlTextOnlyVideo)
	}
}

func TestHandleEmbedRouter(t *testing.T) {
	// Initialize crawler regex
	InitCrawlerDetector()

	// Scenario 1: Non-crawler User Agent (browser)
	// It should respond with a 301 Moved Permanently redirect to Facebook.
	reqBrowser := httptest.NewRequest("GET", "/some/post/path", nil)
	reqBrowser.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")
	recBrowser := httptest.NewRecorder()

	handleEmbedRouter(recBrowser, reqBrowser)

	if recBrowser.Code != http.StatusMovedPermanently {
		t.Errorf("expected status 301 for browser UA, got %d", recBrowser.Code)
	}
	loc := recBrowser.Header().Get("Location")
	if loc != "https://www.facebook.com/some/post/path" {
		t.Errorf("expected location redirect to facebook, got %q", loc)
	}
	if !strings.Contains(recBrowser.Body.String(), "redirecting...") {
		t.Errorf("expected redirect page HTML, got %q", recBrowser.Body.String())
	}

	// Scenario 2: Crawler User Agent
	// For a path that doesn't match any Facebook structure (like "hello"), it falls back to git.facebed.com.
	reqCrawler := httptest.NewRequest("GET", "/hello", nil)
	reqCrawler.Header.Set("User-Agent", "facebookexternalhit/1.1")
	recCrawler := httptest.NewRecorder()

	handleEmbedRouter(recCrawler, reqCrawler)

	if recCrawler.Code != http.StatusOK {
		t.Errorf("expected status 200 for crawler UA fallback, got %d", recCrawler.Code)
	}
	body := recCrawler.Body.String()
	if !strings.Contains(body, "https://git.facebed.com") {
		t.Errorf("expected crawler response to contain fallback url git.facebed.com, got %q", body)
	}

	// Scenario 3: Banned User checks
	globalConfig.BannedUsers = []string{"12345"}
	if !isBannedUser("12345") {
		t.Error("expected 12345 to be banned user")
	}
	if isBannedUser("67890") {
		t.Error("expected 67890 not to be banned user")
	}
}

func TestReelLiveScrape(t *testing.T) {
	// Only run live scrape test if cookies.json is present
	if _, err := os.Stat("cookies.json"); os.IsNotExist(err) {
		t.Skip("Skipping live scrape test because cookies.json is not present")
	}

	// Load cookies
	globalCookieStore.Load()

	// Initialize crawler
	InitCrawlerDetector()

	// Test the specific Facebook Reel path
	postPath := "reel/27286589274295277"
	t.Logf("Fetching and parsing postPath: %s", postPath)

	parser := ReelsParser{}
	post, err := parser.ProcessPost(postPath)
	if err != nil {
		t.Fatalf("Failed to process post: %v", err)
	}

	t.Logf("Successfully parsed post!")
	t.Logf("Author: %s", post.AuthorName)
	t.Logf("URL: %s", post.URL)
	t.Logf("Likes: %s, Comments: %s, Shares: %s", post.Likes, post.Comments, post.Shares)
	t.Logf("Video Link: %s", post.VideoLinks)

	if post.AuthorName != "Alessio" && post.AuthorName != "Chu Hà Nhân" {
		t.Errorf("Expected author name 'Alessio' or 'Chu Hà Nhân', got %q", post.AuthorName)
	}

	if len(post.VideoLinks) == 0 || post.VideoLinks[0] == "" {
		t.Errorf("Expected at least one non-empty video link")
	}

	// Generate full embed HTML
	embedHTML := FormatReelPostEmbed(post, false)
	t.Logf("Generated Embed HTML:\n%s", embedHTML)
}
