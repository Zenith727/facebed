package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/imroc/req/v3"
)

type FacebedError struct {
	Message string
	HTML    string
	URL     string
}

func (e *FacebedError) Error() string {
	return e.Message
}

func GetStealthClient() *req.Client {
	c := req.C()
	c.ImpersonateChrome()
	c.SetTimeout(15 * time.Second)
	return c
}

func GetScrapeHeaders() map[string]string {
	return map[string]string{
		"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/jxl,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"accept-language": "en-US,en;q=0.9",
		"cache-control":   "no-cache",
		"pragma":          "no-cache",
		"priority":        "u=0, i",
		"sec-fetch-mode":  "navigate",
		"sec-fetch-site":  "none",
	}
}

// JsonParser Helpers

func GetJsonBlocks(html string) []map[string]any {
	var blocks []map[string]any
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return blocks
	}

	doc.Find("script[type='application/json']").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if text == "" {
			return
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			blocks = append(blocks, parsed)
		}
	})
	return blocks
}

func FindDashVideoLink(obj any) string {
	var bestURL string
	var maxBandwidth float64 = 0

	var scan func(val any)
	scan = func(val any) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case map[string]any:
			mime, hasMime := v["mime_type"].(string)
			baseURL, hasURL := v["base_url"].(string)
			if hasMime && hasURL && strings.HasPrefix(mime, "video") && baseURL != "" {
				bandwidth := 0.0
				if bwVal, ok := v["bandwidth"]; ok {
					switch bw := bwVal.(type) {
					case float64:
						bandwidth = bw
					case int:
						bandwidth = float64(bw)
					case int64:
						bandwidth = float64(bw)
					}
				}
				if bandwidth > maxBandwidth || bestURL == "" {
					maxBandwidth = bandwidth
					bestURL = baseURL
				}
			}
			for _, child := range v {
				scan(child)
			}
		case []any:
			for _, child := range v {
				scan(child)
			}
		}
	}

	scan(obj)
	return bestURL
}

func ProbePageType(html string, blocks []map[string]any) string {
	hasPostData := false
	for _, block := range blocks {
		if JqHas(block, "i18n_reaction_count") || JqHas(block, "short_form_video_context") || JqHas(block, "unified_reactors") || JqHas(block, "video_owner") || FindDashVideoLink(block) != "" {
			hasPostData = true
			break
		}
	}
	if hasPostData {
		return "has_data"
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err == nil {
		canonical, ok := doc.Find("link[rel='canonical']").Attr("href")
		if ok && regexp.MustCompile(`/login\b`).MatchString(canonical) {
			return "login_wall"
		}
	}
	return "no_data"
}

func FetchPage(postPath string, useCookies bool) (string, []map[string]any, error) {
	urlStr := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(postPath, "/"))
	client := GetStealthClient()
	r := client.R()
	for k, v := range GetScrapeHeaders() {
		r.SetHeader(k, v)
	}

	if useCookies {
		for k, v := range globalCookieStore.GetMap() {
			r.SetCookies(&http.Cookie{Name: k, Value: v})
		}
	}

	resp, err := r.Get(urlStr)
	if err != nil {
		return "", nil, &FacebedError{Message: fmt.Sprintf("Request failed: %v", err), URL: urlStr}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, &FacebedError{Message: fmt.Sprintf("Read failed: %v", err), URL: urlStr}
	}
	body := string(bodyBytes)

	blocks := GetJsonBlocks(body)
	pageType := ProbePageType(body, blocks)
	if pageType == "login_wall" || pageType == "no_data" {
		return body, blocks, &FacebedError{
			Message: fmt.Sprintf("Facebook served a login wall or empty page for %s - content requires authentication", postPath),
			HTML:    body,
			URL:     urlStr,
		}
	}

	return body, blocks, nil
}

func GetPostJson(blocks []map[string]any) (map[string]any, error) {
	var candidates []map[string]any
	for _, block := range blocks {
		if JqHas(block, "i18n_reaction_count") || JqHas(block, "short_form_video_context") {
			candidates = append(candidates, block)
		}
	}

	if len(candidates) == 0 {
		for _, block := range blocks {
			strVal := fmt.Sprintf("%v", block)
			if strings.Contains(strVal, "video") && strings.Contains(strVal, "short_form_video_context") {
				candidates = append(candidates, block)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, &FacebedError{Message: "cannot find post json"}
	}

	scoreBlock := func(b map[string]any) int {
		score := 0
		if JqHas(b, "short_form_video_context") {
			score += 20
		}
		if JqHas(b, "creation_story") {
			score += 10
		}
		if JqHas(b, "comet_sections") {
			score += 5
		}
		if JqHas(b, "group_hoisted_feed") {
			score += 8
		}
		strVal := fmt.Sprintf("%v", b)
		if strings.Contains(strVal, "video_home_www_related_videos_section") || strings.Contains(strVal, "video_home_www_loe_video_permalink_seo_info") {
			score -= 20
		}

		nodeV2 := JqFirst(b, "node_v2")
		if m, ok := nodeV2.(map[string]any); ok {
			if actors, ok := m["actors"].([]any); ok && len(actors) > 0 {
				score += 30
			}
			if _, ok := m["feedback"].(map[string]any); ok {
				score += 15
			}
			if JqHas(m, "comet_sections") || JqHas(m, "creation_story") {
				score += 10
			}
		}

		dataBlob := JqFirst(b, "data")
		if m, ok := dataBlob.(map[string]any); ok {
			if actors, ok := m["actors"].([]any); ok && len(actors) > 0 {
				score += 20
			}
			if _, ok := m["feedback"].(map[string]any); ok {
				score += 10
			}
		}

		if _, hasReq := b["require"]; hasReq && nodeV2 == nil && dataBlob == nil {
			score -= 10
		}
		return score
	}

	sort.Slice(candidates, func(i, j int) bool {
		return scoreBlock(candidates[i]) > scoreBlock(candidates[j])
	})

	return candidates[0], nil
}

func GetGroupName(blocks []map[string]any) string {
	for _, block := range blocks {
		if JqHas(block, "group_member_profiles", "formatted_count_text") {
			groups := JqAll(block, "group")
			for _, gVal := range groups {
				if gMap, ok := gVal.(map[string]any); ok {
					if name, ok := gMap["name"].(string); ok {
						return name
					}
				}
			}
		}
	}
	return ""
}

func GetInteractionCounts(postJSON map[string]any) (likes, comments, shares string) {
	extract := func(fb map[string]any) (string, string, string) {
		r := fmt.Sprintf("%v", JqFirst(fb, "i18n_reaction_count"))
		if r == "<nil>" || r == "0" {
			r = "0"
		}
		
		s := fmt.Sprintf("%v", JqFirst(fb, "i18n_share_count"))
		if s == "<nil>" || s == "0" {
			s = fmt.Sprintf("%v", JqFirst(fb, "share_count"))
			if s == "<nil>" || s == "0" {
				s = "0"
			}
		}

		c := ""
		cVal := fb["total_comment_count"]
		if cVal == nil {
			cVal = JqFirst(fb, "total_comment_count")
		}
		if cVal != nil {
			c = fmt.Sprintf("%v", cVal)
		}

		if c == "" || c == "<nil>" {
			if cri, ok := fb["comment_rendering_instance"].(map[string]any); ok {
				if cmts, ok := cri["comments"].(map[string]any); ok {
					c = fmt.Sprintf("%v", cmts["total_count"])
				}
			}
		}
		if c == "" || c == "<nil>" {
			if ccsr, ok := fb["comments_count_summary_renderer"].(map[string]any); ok {
				if fbInner, ok := ccsr["feedback"].(map[string]any); ok {
					if cri2, ok := fbInner["comment_rendering_instance"].(map[string]any); ok {
						if cmts2, ok := cri2["comments"].(map[string]any); ok {
							c = fmt.Sprintf("%v", cmts2["total_count"])
						}
					}
				}
			}
		}
		if c == "" || c == "<nil>" {
			c = "0"
		}
		return r, c, s
	}

	bestFeedback := func() map[string]any {
		var best map[string]any
		bestReactions := 0
		feedbacks := JqAll(postJSON, "feedback")
		for _, fbVal := range feedbacks {
			fb, ok := fbVal.(map[string]any)
			if !ok {
				continue
			}
			rcVal := fb["i18n_reaction_count"]
			if rcVal != nil {
				if n, err := strconv.Atoi(fmt.Sprintf("%v", rcVal)); err == nil {
					if n > bestReactions {
						bestReactions = n
						best = fb
					}
				}
			} else if best == nil {
				best = fb
			}
		}
		return best
	}

	postFeedback := JqFirst(postJSON, "comet_ufi_summary_and_actions_renderer")
	if m, ok := postFeedback.(map[string]any); ok {
		if fb, ok := m["feedback"].(map[string]any); ok {
			return extract(fb)
		}
	}

	ufiInSections := JqFirst(postJSON, "comet_ufi_summary_and_actions_renderer")
	if m, ok := ufiInSections.(map[string]any); ok {
		if ufiFeedback, ok := m["feedback"].(map[string]any); ok {
			r, c, s := extract(ufiFeedback)
			if r != "0" || c != "0" || s != "0" {
				return r, c, s
			}
		}
	}

	if fb, ok := JqFirst(postJSON, "feedback").(map[string]any); ok {
		if rc := fb["i18n_reaction_count"]; rc != nil {
			return extract(fb)
		}
		if best := bestFeedback(); best != nil {
			return extract(best)
		}
	}

	if best := bestFeedback(); best != nil {
		return extract(best)
	}

	rVal := fmt.Sprintf("%v", JqFirst(postJSON, "i18n_reaction_count"))
	if rVal == "<nil>" {
		rVal = "0"
	}
	sVal := fmt.Sprintf("%v", JqFirst(postJSON, "i18n_share_count"))
	if sVal == "<nil>" {
		sVal = fmt.Sprintf("%v", JqFirst(postJSON, "share_count"))
		if sVal == "<nil>" {
			sVal = "0"
		}
	}

	cVal := JqFirst(postJSON, "total_comment_count")
	c := ""
	if cVal != nil {
		c = fmt.Sprintf("%v", cVal)
	}
	if c == "" || c == "<nil>" {
		if cri, ok := JqFirst(postJSON, "comment_rendering_instance").(map[string]any); ok {
			if cmts, ok := cri["comments"].(map[string]any); ok {
				c = fmt.Sprintf("%v", cmts["total_count"])
			}
		}
	}
	if c == "" || c == "<nil>" {
		if ccsr, ok := JqFirst(postJSON, "comments_count_summary_renderer").(map[string]any); ok {
			if fbInner, ok := ccsr["feedback"].(map[string]any); ok {
				if cri2, ok := fbInner["comment_rendering_instance"].(map[string]any); ok {
					if cmts2, ok := cri2["comments"].(map[string]any); ok {
						c = fmt.Sprintf("%v", cmts2["total_count"])
					}
				}
			}
		}
	}
	if c == "" || c == "<nil>" {
		c = "0"
	}

	return rVal, c, sVal
}

func GetRootNode(postJSON map[string]any) (map[string]any, error) {
	workNormalPost := func() map[string]any {
		dataBlob, ok := JqFirst(postJSON, "data").(map[string]any)
		if !ok {
			if shortForm := JqFirst(postJSON, "short_form_video_context"); shortForm != nil {
				return map[string]any{"creation_story": shortForm}
			}
			return nil
		}
		if _, ok := dataBlob["comet_ufi_summary_and_actions_renderer"]; ok {
			return dataBlob
		} else if nodeV2, ok := dataBlob["node_v2"].(map[string]any); ok {
			if JqHas(nodeV2, "comet_sections") || JqHas(nodeV2, "creation_story") {
				return nodeV2
			}
		} else if node, ok := dataBlob["node"].(map[string]any); ok {
			if JqHas(node, "comet_sections") || JqHas(node, "creation_story") {
				return node
			}
		}
		if shortForm := JqFirst(dataBlob, "short_form_video_context"); shortForm != nil {
			return map[string]any{"creation_story": shortForm}
		}
		return nil
	}

	workGroupPost := func() map[string]any {
		hoistedFeed, ok := JqFirst(postJSON, "group_hoisted_feed").(map[string]any)
		if ok {
			if JqHas(hoistedFeed, "comet_sections") || JqHas(hoistedFeed, "creation_story") {
				return hoistedFeed
			}
			if nodeV2, ok := JqFirst(hoistedFeed, "node_v2").(map[string]any); ok {
				return nodeV2
			}
		}

		dataBlob, ok := JqFirst(postJSON, "data").(map[string]any)
		if ok {
			if group, ok := dataBlob["group"].(map[string]any); ok {
				if JqHas(group, "comet_sections") || JqHas(group, "creation_story") {
					return group
				}
				if nodeV2, ok := JqFirst(group, "node_v2").(map[string]any); ok {
					return nodeV2
				}
			}
		}
		return nil
	}

	for _, method := range []func() map[string]any{workNormalPost, workGroupPost} {
		if ret := method(); ret != nil {
			return ret, nil
		}
	}

	dataBlob, ok := JqFirst(postJSON, "data").(map[string]any)
	if ok {
		if JqHas(dataBlob, "creation_story", "feedback") {
			return dataBlob, nil
		}
		if nodeV2, ok := dataBlob["node_v2"].(map[string]any); ok {
			return nodeV2, nil
		}
	}

	return nil, &FacebedError{Message: "Cannot process post"}
}

func ProcessPost(postPath string) (ParsedPost, error) {
	_, blocks, err := FetchPage(postPath, true)
	if err != nil {
		return ParsedPost{}, err
	}

	postJSON, err := GetPostJson(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	rootNode, err := GetRootNode(postJSON)
	if err != nil {
		return ParsedPost{}, err
	}

	likes, cmts, shares := GetInteractionCounts(rootNode)

	// Resolve Date
	var postDate int64 = -1
	t := JqFirst(rootNode, "creation_time")
	if t == nil {
		t = JqFirst(rootNode, "created_time")
	}
	if t != nil {
		if intVal, err := strconv.ParseInt(fmt.Sprintf("%v", t), 10, 64); err == nil {
			postDate = intVal
		}
	}

	if postDate == -1 {
		for _, block := range blocks {
			t = JqFirst(block, "creation_time")
			if t == nil {
				t = JqFirst(block, "created_time")
			}
			if t != nil {
				if intVal, err := strconv.ParseInt(fmt.Sprintf("%v", t), 10, 64); err == nil {
					postDate = intVal
					break
				}
			}
		}
	}

	storyDict := rootNode
	if content, ok := rootNode["content"].(map[string]any); ok {
		if story, ok := content["story"].(map[string]any); ok {
			storyDict = story
		}
	} else if creationStory, ok := rootNode["creation_story"].(map[string]any); ok {
		storyDict = creationStory
		if owner, ok := rootNode["owner"].(map[string]any); ok {
			if _, hasActors := storyDict["actors"]; !hasActors {
				storyDict["actors"] = []any{owner}
			}
		}
	} else if sections, ok := rootNode["comet_sections"].(map[string]any); ok {
		if content, ok := sections["content"].(map[string]any); ok {
			if story, ok := content["story"].(map[string]any); ok {
				storyDict = story
			}
		}
	}

	story := NewStory(storyDict)
	postURL := story.URL
	if postURL == "" {
		postURL = fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(postPath, "/"))
	}
	postContent := story.GetText()
	postGroupName := GetGroupName(blocks)
	postAuthorName := story.AuthorName

	linkHeader := postAuthorName
	if postGroupName != "" {
		linkHeader += " • " + postGroupName
	}

	if isBannedUser(story.AuthorID) {
		return Banned(postURL), nil
	}

	return ParsedPost{
		AuthorName: linkHeader,
		Text:       strings.TrimSpace(postContent),
		ImageLinks: story.ImageLinks,
		URL:        postURL,
		Date:       postDate,
		Likes:      likes,
		Comments:   cmts,
		Shares:     shares,
		VideoLinks: story.VideoLinks,
	}, nil
}

// SinglePhotoParser

type SinglePhotoParser struct{}

func (s SinglePhotoParser) GetContentNode(blocks []map[string]any) (map[string]any, error) {
	for _, block := range blocks {
		if JqHas(block, "message_preferred_body", "container_story") {
			if data, ok := JqFirst(block, "data").(map[string]any); ok {
				return data, nil
			}
		}
	}
	return nil, &FacebedError{Message: "Cannot process post (cn)"}
}

func (s SinglePhotoParser) GetInteractionsNode(blocks []map[string]any) (map[string]any, error) {
	for _, block := range blocks {
		if JqHas(block, "comet_ufi_summary_and_actions_renderer") {
			return block, nil
		}
	}
	return nil, &FacebedError{Message: "Cannot process post (in)"}
}

func (s SinglePhotoParser) GetSingleImage(blocks []map[string]any) (string, error) {
	for _, block := range blocks {
		if JqHas(block, "prefetch_uris_v2") {
			if uris, ok := JqFirst(block, "prefetch_uris_v2").([]any); ok && len(uris) > 0 {
				if uMap, ok := uris[0].(map[string]any); ok {
					if uri, ok := uMap["uri"].(string); ok {
						return uri, nil
					}
				}
			}
		}
	}
	return "", &FacebedError{Message: "cannot find single image"}
}

func (s SinglePhotoParser) ProcessPost(postPath string) (ParsedPost, error) {
	_, blocks, err := FetchPage(postPath, true)
	if err != nil {
		return ParsedPost{}, err
	}

	contentNode, err := s.GetContentNode(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	interactionNode, err := s.GetInteractionsNode(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	postText := ""
	if message, ok := contentNode["message"].(map[string]any); ok {
		postText, _ = message["text"].(string)
	}

	postAuthor := ""
	if owner, ok := contentNode["owner"].(map[string]any); ok {
		postAuthor, _ = owner["name"].(string)
	}

	var postDate int64 = 0
	if dtVal, ok := contentNode["created_time"]; ok {
		if intVal, err := strconv.ParseInt(fmt.Sprintf("%v", dtVal), 10, 64); err == nil {
			postDate = intVal
		}
	}

	likes, cmts, shares := GetInteractionCounts(interactionNode)
	imageURL, err := s.GetSingleImage(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	urlStr := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(postPath, "/"))

	return ParsedPost{
		AuthorName: postAuthor,
		Text:       strings.TrimSpace(postText),
		ImageLinks: []string{imageURL},
		URL:        urlStr,
		Date:       postDate,
		Likes:      likes,
		Comments:   cmts,
		Shares:     shares,
		VideoLinks: []string{},
	}, nil
}

// PhotocomParser

type PhotocomParser struct{}

func (p PhotocomParser) GetContentNode(blocks []map[string]any) (map[string]any, error) {
	for _, block := range blocks {
		if JqHas(block, "attached_comment") && !JqHas(block, "unified_reactors") {
			if res, ok := JqFirst(block, "result").(map[string]any); ok {
				return res, nil
			}
		}
	}
	return nil, &FacebedError{Message: "Cannot process photocom (cn)"}
}

func (p PhotocomParser) GetReactionCount(blocks []map[string]any) (int, error) {
	for _, block := range blocks {
		if JqHas(block, "attached_comment", "unified_reactors") {
			if reactors, ok := JqFirst(block, "unified_reactors").(map[string]any); ok {
				if countVal, ok := reactors["count"]; ok {
					if n, err := strconv.Atoi(fmt.Sprintf("%v", countVal)); err == nil {
						return n, nil
					}
				}
			}
		}
	}
	return 0, &FacebedError{Message: "Cannot process photocom (rc)"}
}

func (p PhotocomParser) GetAttachedImageAndUrl(blocks []map[string]any) (string, string, error) {
	for _, block := range blocks {
		if JqHas(block, "attached_comment", "unified_reactors") {
			if cur, ok := JqFirst(block, "currMedia").(map[string]any); ok {
				var imageURI, fbURL string
				if img, ok := cur["image"].(map[string]any); ok {
					imageURI, _ = img["uri"].(string)
				}
				if comment, ok := cur["attached_comment"].(map[string]any); ok {
					if feedback, ok := comment["feedback"].(map[string]any); ok {
						fbURL, _ = feedback["url"].(string)
					}
				}
				return imageURI, fbURL, nil
			}
		}
	}
	return "", "", &FacebedError{Message: "Cannot process photocom (iau)"}
}

func (p PhotocomParser) ProcessPost(postPath string) (ParsedPost, error) {
	_, blocks, err := FetchPage(postPath, true)
	if err != nil {
		return ParsedPost{}, err
	}

	contentNode, err := p.GetContentNode(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	var postText string
	if data, ok := contentNode["data"].(map[string]any); ok {
		if comment, ok := data["attached_comment"].(map[string]any); ok {
			if body, ok := comment["preferred_body"].(map[string]any); ok {
				postText, _ = body["text"].(string)
			}
		}
	}

	var opName string
	if data, ok := contentNode["data"].(map[string]any); ok {
		if owner, ok := data["owner"].(map[string]any); ok {
			name, _ := owner["name"].(string)
			opName = name + " (💬)"
		}
	}

	var postTime int64 = 0
	if data, ok := contentNode["data"].(map[string]any); ok {
		if createdVal, ok := data["created_time"]; ok {
			if intVal, err := strconv.ParseInt(fmt.Sprintf("%v", createdVal), 10, 64); err == nil {
				postTime = intVal
			}
		}
	}

	postImage, postURL, err := p.GetAttachedImageAndUrl(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	reactionCount, err := p.GetReactionCount(blocks)
	if err != nil {
		reactionCount = 0
	}

	return ParsedPost{
		AuthorName: opName,
		Text:       postText,
		ImageLinks: []string{postImage},
		URL:        postURL,
		Date:       postTime,
		Likes:      HumanFormat(strconv.Itoa(reactionCount)),
		Comments:   "null",
		Shares:     "null",
		VideoLinks: []string{},
	}, nil
}

// ReelsParser

type ReelsParser struct{}

func ExtractVideoID(postPath string) string {
	reelReg := regexp.MustCompile(`(?i)reel/([0-9]+)`)
	if matches := reelReg.FindStringSubmatch(postPath); len(matches) > 1 {
		return matches[1]
	}
	if strings.Contains(postPath, "v=") {
		watchReg := regexp.MustCompile(`(?i)[?&]v=([0-9]+)`)
		if matches := watchReg.FindStringSubmatch(postPath); len(matches) > 1 {
			return matches[1]
		}
	}
	digitsReg := regexp.MustCompile(`([0-9]{10,})`)
	if matches := digitsReg.FindStringSubmatch(postPath); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func FindTargetProgressiveURLs(val any, videoID string) []string {
	var urls []string
	var scan func(v any)
	scan = func(v any) {
		if v == nil {
			return
		}
		switch node := v.(type) {
		case map[string]any:
			idVal, hasID := node["id"]
			videoIDVal, hasVideoID := node["video_id"]
			idStr := ""
			if hasID {
				idStr = fmt.Sprintf("%v", idVal)
			} else if hasVideoID {
				idStr = fmt.Sprintf("%v", videoIDVal)
			}

			if idStr == videoID {
				if progURLs, ok := node["progressive_urls"].([]any); ok && len(progURLs) > 0 {
					for _, item := range progURLs {
						if m, ok := item.(map[string]any); ok {
							if pURL, ok := m["progressive_url"].(string); ok && pURL != "" {
								urls = append(urls, pURL)
							}
						}
					}
				}
				if videoNode, ok := node["videoDeliveryLegacyFields"].(map[string]any); ok {
					for _, key := range []string{"browser_native_hd_url", "browser_native_sd_url"} {
						if link, ok := videoNode[key].(string); ok && link != "" {
							urls = append(urls, link)
						}
					}
				}
				if len(urls) > 0 {
					return
				}
			}
			for _, child := range node {
				scan(child)
			}
		case []any:
			for _, item := range node {
				scan(item)
			}
		}
	}
	scan(val)
	return urls
}

func FindTargetDashVideoLink(val any, videoID string) string {
	var bestURL string
	var maxBandwidth float64 = 0

	var scan func(v any)
	scan = func(v any) {
		if v == nil {
			return
		}
		switch node := v.(type) {
		case map[string]any:
			vID, hasVid := node["video_id"]
			if hasVid && fmt.Sprintf("%v", vID) == videoID {
				if reps, ok := node["representations"].([]any); ok {
					for _, rep := range reps {
						if rMap, ok := rep.(map[string]any); ok {
							mime, _ := rMap["mime_type"].(string)
							baseURL, _ := rMap["base_url"].(string)
							if strings.HasPrefix(mime, "video") && baseURL != "" {
								bandwidth := 0.0
								if bwVal, ok := rMap["bandwidth"]; ok {
									switch bw := bwVal.(type) {
									case float64:
										bandwidth = bw
									case int:
										bandwidth = float64(bw)
									case int64:
										bandwidth = float64(bw)
									}
								}
								if bandwidth > maxBandwidth || bestURL == "" {
									maxBandwidth = bandwidth
									bestURL = baseURL
								}
							}
						}
					}
				}
				if bestURL != "" {
					return
				}
			}
			for _, child := range node {
				scan(child)
			}
		case []any:
			for _, item := range node {
				scan(item)
			}
		}
	}

	scan(val)
	return bestURL
}

func GetReelsVideoLinkFromNode(node map[string]any) (string, error) {
	// Try legacy videoDeliveryLegacyFields
	videoNode := JqFirst(node, "videoDeliveryLegacyFields")
	for _, key := range []string{"browser_native_hd_url", "browser_native_sd_url"} {
		if link, ok := JqFirst(videoNode, key).(string); ok && link != "" {
			return link, nil
		}
	}
	// Fall back to recursive search for DASH video links inside this node
	if link := FindDashVideoLink(node); link != "" {
		return link, nil
	}
	return "", &FacebedError{Message: "Invalid reels link (vn)"}
}

func (r ReelsParser) GetVideoLink(blocks []map[string]any, targetVideoID string) (string, error) {
	if targetVideoID != "" {
		var sdLink, hdLink string
		for _, block := range blocks {
			urls := FindTargetProgressiveURLs(block, targetVideoID)
			if len(urls) > 0 {
				var scanProg func(v any)
				scanProg = func(v any) {
					if v == nil {
						return
					}
					switch node := v.(type) {
					case map[string]any:
						idVal, hasID := node["id"]
						videoIDVal, hasVideoID := node["video_id"]
						idStr := ""
						if hasID {
							idStr = fmt.Sprintf("%v", idVal)
						} else if hasVideoID {
							idStr = fmt.Sprintf("%v", videoIDVal)
						}

						if idStr == targetVideoID {
							if progList, ok := node["progressive_urls"].([]any); ok {
								for _, item := range progList {
									if itemMap, ok := item.(map[string]any); ok {
										pURL, _ := itemMap["progressive_url"].(string)
										if pURL != "" {
											quality := ""
											if meta, ok := itemMap["metadata"].(map[string]any); ok {
												quality, _ = meta["quality"].(string)
											}
											if quality == "HD" {
												hdLink = pURL
											} else if quality == "SD" {
												sdLink = pURL
											} else if sdLink == "" {
												sdLink = pURL
											}
										}
									}
								}
							}
						}
						for _, child := range node {
							scanProg(child)
						}
					case []any:
						for _, item := range node {
							scanProg(item)
						}
					}
				}
				scanProg(block)
				if hdLink != "" {
					return hdLink, nil
				}
				if sdLink != "" {
					return sdLink, nil
				}
				return urls[0], nil
			}
		}

		for _, block := range blocks {
			var scanLegacy func(v any) string
			scanLegacy = func(v any) string {
				if v == nil {
					return ""
				}
				switch node := v.(type) {
				case map[string]any:
					idVal, hasID := node["id"]
					videoIDVal, hasVideoID := node["video_id"]
					idStr := ""
					if hasID {
						idStr = fmt.Sprintf("%v", idVal)
					} else if hasVideoID {
						idStr = fmt.Sprintf("%v", videoIDVal)
					}

					if idStr == targetVideoID {
						if videoNode, ok := node["videoDeliveryLegacyFields"].(map[string]any); ok {
							for _, key := range []string{"browser_native_hd_url", "browser_native_sd_url"} {
								if link, ok := videoNode[key].(string); ok && link != "" {
									return link
								}
							}
						}
						for _, key := range []string{"browser_native_hd_url", "browser_native_sd_url"} {
							if link, ok := node[key].(string); ok && link != "" {
								return link
							}
						}
					}
					for _, child := range node {
						if res := scanLegacy(child); res != "" {
							return res
						}
					}
				case []any:
					for _, item := range node {
						if res := scanLegacy(item); res != "" {
							return res
						}
					}
				}
				return ""
			}
			if link := scanLegacy(block); link != "" {
				return link, nil
			}
		}

		for _, block := range blocks {
			if link := FindTargetDashVideoLink(block, targetVideoID); link != "" {
				return link, nil
			}
		}
	}

	for _, block := range blocks {
		if JqHas(block, "browser_native_hd_url") || JqHas(block, "browser_native_sd_url") || FindDashVideoLink(block) != "" {
			link, err := GetReelsVideoLinkFromNode(block)
			if err == nil && link != "" {
				return link, nil
			}
		}
	}
	return "", &FacebedError{Message: "Invalid reels link (vn)"}
}

func (r ReelsParser) GetContentNode(blocks []map[string]any) (map[string]any, error) {
	for _, block := range blocks {
		if JqHas(block, "creation_story") && (JqHas(block, "browser_native_sd_url") || FindDashVideoLink(block) != "") {
			if node, ok := JqFirst(block, "creation_story").(map[string]any); ok {
				return node, nil
			}
		}
		if JqHas(block, "short_form_video_context") {
			if node, ok := JqFirst(block, "short_form_video_context").(map[string]any); ok {
				return node, nil
			}
		}
	}
	// Fallback to any block that has video_owner info
	for _, block := range blocks {
		if JqHas(block, "video_owner") {
			return block, nil
		}
	}
	return nil, &FacebedError{Message: "Invalid reels link (cn)"}
}

func (r ReelsParser) GetReactionCounts(blocks []map[string]any, isIG bool, videoID string) (likes, cmts, shares string, err error) {
	var matchedBlocks []map[string]any
	for _, block := range blocks {
		if JqHas(block, "unified_reactors") {
			ids := JqAll(block, "id")
			for _, idVal := range ids {
				if fmt.Sprintf("%v", idVal) == videoID {
					matchedBlocks = append(matchedBlocks, block)
					break
				}
			}
		}
	}

	if len(matchedBlocks) == 0 {
		return "", "", "", &FacebedError{Message: "Cannot process post (cn)"}
	}

	block := matchedBlocks[0]
	firstFB, _ := JqFirst(block, "feedback").(map[string]any)
	lastFB, _ := JqLast(block, "feedback").(map[string]any)

	if strings.Contains(fmt.Sprintf("%v", firstFB), "cross_universe_feedback_info") {
		firstFB, lastFB = lastFB, firstFB
	}

	igCmts := ""
	if cross, ok := lastFB["cross_universe_feedback_info"].(map[string]any); ok {
		igCmts = fmt.Sprintf("%v", cross["ig_comment_count"])
	}

	likesVal := "0"
	if reactors, ok := firstFB["unified_reactors"].(map[string]any); ok {
		likesVal = fmt.Sprintf("%v", reactors["count"])
	}

	cmtsVal := ""
	if isIG {
		cmtsVal = igCmts
	} else {
		cmtsVal = fmt.Sprintf("%v", lastFB["total_comment_count"])
	}

	sharesVal := fmt.Sprintf("%v", lastFB["share_count_reduced"])

	return HumanFormat(likesVal), HumanFormat(cmtsVal), HumanFormat(sharesVal), nil
}

func (r ReelsParser) ProcessPost(postPath string) (ParsedPost, error) {
	_, blocks, err := FetchPage(postPath, true)
	if err != nil {
		return ParsedPost{}, err
	}

	contentNode, err := r.GetContentNode(blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	videoID := ExtractVideoID(postPath)
	if videoID == "" {
		if val, ok := contentNode["id"]; ok {
			videoID = fmt.Sprintf("%v", val)
		} else if vidMap, ok := contentNode["video"].(map[string]any); ok {
			if val, ok := vidMap["id"]; ok {
				videoID = fmt.Sprintf("%v", val)
			}
		}
		if videoID == "" {
			if firstID := JqFirst(contentNode, "id"); firstID != nil {
				videoID = fmt.Sprintf("%v", firstID)
			}
		}
	}

	videoLink, err := r.GetVideoLink(blocks, videoID)
	if err != nil {
		return ParsedPost{}, err
	}

	var ownerInfo map[string]any
	if owner, ok := contentNode["video_owner"].(map[string]any); ok {
		ownerInfo = owner
	} else if sfvc, ok := contentNode["short_form_video_context"].(map[string]any); ok {
		if owner, ok := sfvc["video_owner"].(map[string]any); ok {
			ownerInfo = owner
		}
	} else if owner, ok := JqFirst(contentNode, "owner").(map[string]any); ok {
		ownerInfo = owner
	} else if actors, ok := JqFirst(contentNode, "actors").([]any); ok && len(actors) > 0 {
		if firstActor, ok := actors[0].(map[string]any); ok {
			ownerInfo = firstActor
		}
	}

	if ownerInfo != nil {
		name, _ := ownerInfo["name"].(string)
		username, _ := ownerInfo["username"].(string)
		if name == "" && username == "" {
			ownerInfo = nil
		}
	}

	// Fallback logic: look for block with videoID and retrieve owner or actors with non-empty name/username
	if ownerInfo == nil {
		for _, block := range blocks {
			strVal := fmt.Sprintf("%v", block)
			if strings.Contains(strVal, videoID) {
				if owner, ok := JqFirst(block, "owner").(map[string]any); ok {
					name, _ := owner["name"].(string)
					username, _ := owner["username"].(string)
					if name != "" || username != "" {
						ownerInfo = owner
						break
					}
				}
				if actors, ok := JqFirst(block, "actors").([]any); ok && len(actors) > 0 {
					if firstActor, ok := actors[0].(map[string]any); ok {
						name, _ := firstActor["name"].(string)
						username, _ := firstActor["username"].(string)
						if name != "" || username != "" {
							ownerInfo = firstActor
							break
						}
					}
				}
			}
		}
	}

	if ownerInfo == nil {
		return ParsedPost{}, &FacebedError{Message: "could not extract video owner info"}
	}

	typeName, _ := ownerInfo["__typename"].(string)
	isIG := strings.HasPrefix(typeName, "InstagramUser")

	opName := ""
	if isIG {
		opName = "📷 @"
		username, _ := ownerInfo["username"].(string)
		opName += username
	} else {
		name, _ := ownerInfo["name"].(string)
		opName += name
	}

	postURL := ""
	if sfvc, ok := contentNode["short_form_video_context"].(map[string]any); ok {
		postURL, _ = sfvc["shareable_url"].(string)
	}
	if postURL == "" {
		postURL = fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(postPath, "/"))
	}

	var postDate int64 = -1
	if ctVal, ok := contentNode["creation_time"]; ok {
		if intVal, err := strconv.ParseInt(fmt.Sprintf("%v", ctVal), 10, 64); err == nil {
			postDate = intVal
		}
	}
	if postDate == -1 {
		for _, block := range blocks {
			dtVal := JqFirst(block, "creation_time")
			if dtVal == nil {
				dtVal = JqFirst(block, "created_time")
			}
			if dtVal != nil {
				if intVal, err := strconv.ParseInt(fmt.Sprintf("%v", dtVal), 10, 64); err == nil {
					postDate = intVal
					break
				}
			}
		}
	}

	postText := ""
	if msg, ok := contentNode["message"].(map[string]any); ok {
		postText, _ = msg["text"].(string)
	}

	likes, cmts, shares, err := r.GetReactionCounts(blocks, isIG, videoID)
	if err != nil {
		likes, cmts, shares = "0", "0", "0"
	}

	ownerID := fmt.Sprintf("%v", ownerInfo["id"])
	if isBannedUser(ownerID) {
		return Banned(postURL), nil
	}

	return ParsedPost{
		AuthorName: opName,
		Text:       strings.TrimSpace(postText),
		ImageLinks: []string{},
		URL:        postURL,
		Date:       postDate,
		Likes:      likes,
		Comments:   cmts,
		Shares:     shares,
		VideoLinks: []string{videoLink},
	}, nil
}

// VideoWatchParser

type VideoWatchParser struct{}

func (v VideoWatchParser) GetOpName(blocks []map[string]any) (string, error) {
	for _, block := range blocks {
		if JqHas(block, "is_additional_profile_plus") {
			if owner, ok := JqFirst(block, "owner").(map[string]any); ok {
				if name, ok := owner["name"].(string); ok {
					return name, nil
				}
			}
		}
	}
	for _, block := range blocks {
		if owner, ok := JqFirst(block, "owner").(map[string]any); ok {
			if name, ok := owner["name"].(string); ok {
				return name, nil
			}
		}
	}
	return "", &FacebedError{Message: "Invalid watch link (opn)"}
}

func (v VideoWatchParser) GetContentNode(html string, blocks []map[string]any) (map[string]any, error) {
	for _, block := range blocks {
		if JqHas(block, "comment_rendering_instance", "video_view_count_renderer") {
			if res, ok := JqFirst(block, "result").(map[string]any); ok {
				if data, ok := res["data"].(map[string]any); ok {
					return data, nil
				}
			}
		}
	}
	// Fallback to any block that has video_owner info
	for _, block := range blocks {
		if JqHas(block, "video_owner") {
			return block, nil
		}
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err == nil {
		canonical, ok := doc.Find("link[rel='canonical']").Attr("href")
		if ok && regexp.MustCompile(`https?://[^/]+/watch/?$`).MatchString(canonical) {
			return nil, &FacebedError{Message: "Facebook served generic watch feed instead of specific video"}
		}
	}

	return nil, &FacebedError{Message: "Invalid watch link (cn)"}
}

func (v VideoWatchParser) GetDate(blocks []map[string]any) (int64, error) {
	for _, block := range blocks {
		if JqHas(block, "creation_time") {
			if dtVal, ok := JqFirst(block, "creation_time").(float64); ok {
				return int64(dtVal), nil
			}
			// fallback string parsing
			if dtStr := fmt.Sprintf("%v", JqFirst(block, "creation_time")); dtStr != "<nil>" {
				if intVal, err := strconv.ParseInt(dtStr, 10, 64); err == nil {
					return intVal, nil
				}
			}
		}
	}
	return 0, &FacebedError{Message: "cannot find date"}
}

func (v VideoWatchParser) ProcessPost(postPath string) (ParsedPost, error) {
	body, blocks, err := FetchPage(postPath, true)
	if err != nil {
		return ParsedPost{}, err
	}

	contentNode, err := v.GetContentNode(body, blocks)
	if err != nil {
		return ParsedPost{}, err
	}

	videoID := ExtractVideoID(postPath)
	if videoID == "" {
		if val, ok := contentNode["id"]; ok {
			videoID = fmt.Sprintf("%v", val)
		} else if vidMap, ok := contentNode["video"].(map[string]any); ok {
			if val, ok := vidMap["id"]; ok {
				videoID = fmt.Sprintf("%v", val)
			}
		}
		if videoID == "" {
			if firstID := JqFirst(contentNode, "id"); firstID != nil {
				videoID = fmt.Sprintf("%v", firstID)
			}
		}
	}

	reels := ReelsParser{}
	videoLink, err := reels.GetVideoLink(blocks, videoID)
	if err != nil {
		return ParsedPost{}, err
	}

	urlStr := fmt.Sprintf("https://www.facebook.com/%s", strings.TrimPrefix(postPath, "/"))
	opName, err := v.GetOpName(blocks)
	if err != nil {
		opName = ""
	}

	postText := ""
	if title, ok := contentNode["title"].(map[string]any); ok {
		postText, _ = title["text"].(string)
	}
	if postText == "" {
		if msg, ok := JqFirst(contentNode, "message").(map[string]any); ok {
			postText, _ = msg["text"].(string)
		}
	}

	likes := "0"
	cmts := "0"
	shares := "null"

	if fb, ok := contentNode["feedback"].(map[string]any); ok {
		if rc, ok := fb["reaction_count"].(map[string]any); ok {
			likes = HumanFormat(fmt.Sprintf("%v", rc["count"]))
		}
		cmts = HumanFormat(fmt.Sprintf("%v", fb["total_comment_count"]))
	}

	postDate, err := v.GetDate(blocks)
	if err != nil {
		postDate = -1
	}

	return ParsedPost{
		AuthorName: opName,
		Text:       strings.TrimSpace(postText),
		ImageLinks: []string{},
		URL:        urlStr,
		Date:       postDate,
		Likes:      likes,
		Comments:   cmts,
		Shares:     shares,
		VideoLinks: []string{videoLink},
	}, nil
}
