package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ParsedPost struct {
	AuthorName string
	Text       string
	ImageLinks []string
	URL        string
	Date       int64
	Likes      string
	Comments   string
	Shares     string
	VideoLinks []string
}

type Story struct {
	AuthorName    string
	Text          string
	ImageLinks    []string
	VideoLinks    []string
	URL           string
	AuthorID      string
	AttachedStory *Story
}

func NewStory(storyJSON map[string]any) *Story {
	var nodeV2 map[string]any

	if _, ok := storyJSON["actors"]; ok {
		nodeV2 = storyJSON
	} else {
		// Check if Comet sections or creation story is in node_v2
		v2 := JqFirst(storyJSON, "node_v2")
		if m, ok := v2.(map[string]any); ok {
			nodeV2 = m
		} else {
			nodeV2 = make(map[string]any)
		}
	}

	s := &Story{}

	// Resolve Author Name
	if actors, ok := storyJSON["actors"].([]any); ok && len(actors) > 0 {
		if actorMap, ok := actors[0].(map[string]any); ok {
			s.AuthorName, _ = actorMap["name"].(string)
		}
	} else if actors, ok := nodeV2["actors"].([]any); ok && len(actors) > 0 {
		if actorMap, ok := actors[0].(map[string]any); ok {
			s.AuthorName, _ = actorMap["name"].(string)
		}
	} else if name, ok := nodeV2["name"].(string); ok && len(name) > 2 {
		s.AuthorName = name
	} else if shortName, ok := nodeV2["short_name"].(string); ok {
		s.AuthorName = shortName
	} else if name, ok := storyJSON["name"].(string); ok && len(name) > 2 {
		s.AuthorName = name
	} else {
		if name, ok := JqFirst(storyJSON, "name").(string); ok {
			s.AuthorName = name
		} else if locName, ok := JqFirst(storyJSON, "localized_name").(string); ok {
			s.AuthorName = locName
		}
	}

	// Resolve Text
	if message, ok := storyJSON["message"].(map[string]any); ok {
		s.Text, _ = message["text"].(string)
	} else if message, ok := storyJSON["message"].(map[string]any); ok {
		s.Text, _ = message["text"].(string)
	} else if text, ok := storyJSON["text"].(string); ok {
		s.Text = text
	} else {
		if text, ok := JqFirst(storyJSON, "text").(string); ok {
			s.Text = text
		}
	}

	// Resolve media links
	s.ImageLinks = getPostImageLinks(storyJSON)
	s.VideoLinks = getPostVideoLinks(storyJSON)

	// Resolve URL
	if u, ok := storyJSON["wwwURL"].(string); ok && u != "" {
		s.URL = u
	} else if u, ok := nodeV2["wwwURL"].(string); ok && u != "" {
		s.URL = u
	} else if u, ok := storyJSON["url"].(string); ok && u != "" {
		s.URL = u
	}

	// Resolve Author ID
	if actors, ok := storyJSON["actors"].([]any); ok && len(actors) > 0 {
		if actorMap, ok := actors[0].(map[string]any); ok {
			if idVal, ok := actorMap["id"]; ok {
				s.AuthorID = fmt.Sprintf("%v", idVal)
			}
		}
	} else if actors, ok := nodeV2["actors"].([]any); ok && len(actors) > 0 {
		if actorMap, ok := actors[0].(map[string]any); ok {
			if idVal, ok := actorMap["id"]; ok {
				s.AuthorID = fmt.Sprintf("%v", idVal)
			}
		}
	} else if idVal, ok := nodeV2["id"]; ok {
		s.AuthorID = fmt.Sprintf("%v", idVal)
	} else {
		if idVal := JqFirst(storyJSON, "id"); idVal != nil {
			s.AuthorID = fmt.Sprintf("%v", idVal)
		}
	}

	// Resolve Attached Story
	if attached, ok := storyJSON["attached_story"].(map[string]any); ok {
		if _, okActors := attached["actors"]; okActors {
			s.AttachedStory = NewStory(attached)
			// Append attached story media
			for _, img := range s.AttachedStory.ImageLinks {
				if !contains(s.ImageLinks, img) {
					s.ImageLinks = append(s.ImageLinks, img)
				}
			}
			for _, vid := range s.AttachedStory.VideoLinks {
				if !contains(s.VideoLinks, vid) {
					s.VideoLinks = append(s.VideoLinks, vid)
				}
			}
		}
	}

	return s
}

func (s *Story) GetText() string {
	text := s.Text
	if s.AttachedStory != nil {
		text += fmt.Sprintf("\n╰┈➤ %s\n%s", s.AttachedStory.AuthorName, s.AttachedStory.Text)
	}
	return text
}

func getPostVideoLinks(postJSON map[string]any) []string {
	var videoLinks []string
	attachments := JqAll(postJSON, "attachment")
	for _, attVal := range attachments {
		attMap, ok := attVal.(map[string]any)
		if !ok {
			continue
		}
		// ReelsParser has helper to parse video link
		link, err := GetReelsVideoLinkFromNode(attMap)
		if err == nil && link != "" && !contains(videoLinks, link) {
			videoLinks = append(videoLinks, link)
		}
	}
	return videoLinks
}

func getPostImageLinks(postJSON map[string]any) []string {
	attachments := JqAll(postJSON, "attachment")
	for _, attVal := range attachments {
		attMap, ok := attVal.(map[string]any)
		if !ok {
			continue
		}

		// Look for subattachments
		var subset map[string]any
		maxImageCount := -1
		for k, v := range attMap {
			if strings.HasSuffix(k, "subattachments") {
				if subMap, ok := v.(map[string]any); ok {
					if nodes, ok := subMap["nodes"].([]any); ok {
						if len(nodes) > maxImageCount {
							maxImageCount = len(nodes)
							subset = subMap
						}
					}
				}
			}
		}

		if subset != nil {
			var images []string
			viewerImages := JqAll(subset, "viewer_image")
			for _, imgVal := range viewerImages {
				if imgMap, ok := imgVal.(map[string]any); ok {
					if uri, ok := imgMap["uri"].(string); ok {
						images = append(images, uri)
					}
				}
			}
			if len(images) > 0 {
				return images
			}
		} else if _, hasMedia := attMap["media"]; hasMedia && !strings.Contains(fmt.Sprintf("%v", attMap), "__typename: Sticker") {
			var images []string
			photoImages := JqAll(attMap, "photo_image")
			for _, imgVal := range photoImages {
				if imgMap, ok := imgVal.(map[string]any); ok {
					if uri, ok := imgMap["uri"].(string); ok {
						images = append(images, uri)
					}
				}
			}
			if len(images) > 0 {
				return images
			}
		}
	}

	oneImg := fallbackGetImageLink(postJSON)
	if oneImg != "" {
		return []string{oneImg}
	}
	return []string{}
}

func fallbackGetImageLink(postJSON map[string]any) string {
	renderers := JqAll(postJSON, "comet_photo_attachment_resolution_renderer")
	for _, rendVal := range renderers {
		if rendMap, ok := rendVal.(map[string]any); ok {
			if image, ok := rendMap["image"].(map[string]any); ok {
				if uri, ok := image["uri"].(string); ok {
					return uri
				}
			}
		}
	}
	return ""
}

func ResolveShareLink(path string) string {
	urlStr := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(path, "/"))
	log.Printf("Resolving share link: %s", urlStr)

	// Use standard http Client with NO custom user-agent so that Facebook follows redirects successfully.
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		log.Printf("failed to create redirect request: %v", err)
		return ""
	}

	// Accept headers similar to requests
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("redirect check failed: %v", err)
		return ""
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	log.Printf("Resolved to: %s", finalURL)

	// If resolved URL is a login redirect containing "next" parameter, extract it
	if strings.Contains(finalURL, "/login/") || strings.Contains(finalURL, "/login.php") {
		u, err := url.Parse(finalURL)
		if err == nil {
			nextVal := u.Query().Get("next")
			if nextVal != "" {
				log.Printf("Extracted redirect 'next' target: %s", nextVal)
				finalURL = nextVal
			}
		}
	}

	if strings.Contains(finalURL, "/share/") {
		log.Println("Warning: Share link still redirects to share page")
		return ""
	}

	for _, base := range []string{"https://www.facebook.com", "https://web.facebook.com"} {
		if strings.HasPrefix(finalURL, base+"/") {
			return strings.TrimPrefix(finalURL, base+"/")
		}
	}
	return finalURL
}

func Banned(url string) ParsedPost {
	WarnWebhook(fmt.Sprintf("banned embed attempted %q", url), nil, "", nil)
	return ParsedPost{
		AuthorName: "Banned",
		Text:       "This user is banned by the operators of this embed server",
		ImageLinks: []string{},
		URL:        "https://banned.facebook.com",
		Date:       -1,
		Likes:      "null",
		Comments:   "null",
		Shares:     "null",
		VideoLinks: []string{},
	}
}

// JQ Emulation Helpers

func JqEnumerate(obj any) []map[string]any {
	var result []map[string]any

	var collect func(val any)
	collect = func(val any) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case map[string]any:
			result = append(result, v)
			// Depth-first search matching Python custom collector order
			for _, child := range v {
				if _, ok := child.([]any); ok {
					collect(child)
				}
			}
			for _, child := range v {
				if _, ok := child.(map[string]any); ok {
					collect(child)
				}
			}
			for _, child := range v {
				if _, ok := child.(map[string]any); !ok {
					if _, okArr := child.([]any); !okArr {
						collect(child)
					}
				}
			}
		case []any:
			for _, item := range v {
				if _, ok := item.(map[string]any); ok {
					collect(item)
				}
			}
			for _, item := range v {
				if _, ok := item.([]any); ok {
					collect(item)
				}
			}
			for _, item := range v {
				if _, ok := item.(map[string]any); !ok {
					if _, okArr := item.([]any); !okArr {
						collect(item)
					}
				}
			}
		}
	}

	collect(obj)
	return result
}

func JqIterate(obj any, key string, first bool) []any {
	var result []any
	for _, m := range JqEnumerate(obj) {
		if val, ok := m[key]; ok {
			if first {
				return []any{val}
			}
			result = append(result, val)
		}
	}
	return result
}

func JqAll(obj any, key string) []any {
	return JqIterate(obj, key, false)
}

func JqFirst(obj any, key string) any {
	res := JqIterate(obj, key, true)
	if len(res) > 0 {
		return res[0]
	}
	return nil
}

func JqHas(obj any, keys ...string) bool {
	for _, k := range keys {
		found := false
		for _, m := range JqEnumerate(obj) {
			if _, ok := m[k]; ok {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func JqLast(obj any, key string) any {
	res := JqIterate(obj, key, false)
	if len(res) > 0 {
		return res[len(res)-1]
	}
	return nil
}

func contains(arr []string, item string) bool {
	for _, x := range arr {
		if x == item {
			return true
		}
	}
	return false
}

func isBannedUser(authorID string) bool {
	for _, banned := range globalConfig.BannedUsers {
		if banned == authorID {
			return true
		}
	}
	return false
}
