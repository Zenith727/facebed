package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Escape special URI characters similar to python's urllib.parse.quote custom behaviour
func EscapeURI(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte("<>\\\"'#%{}[]|\\\\^~`", c) >= 0 {
			sb.WriteString(url.QueryEscape(string(c)))
		} else {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func GetCredit() string {
	return "facebed on GOlang by gemini flash 3.5 with zenith"
}

func TimestampToStr(ts int64) string {
	if ts < 0 {
		return ""
	}
	// Setup timezone location
	loc := time.FixedZone("UTC", globalConfig.Timezone*3600)
	t := time.Unix(ts, 0).In(loc)
	_, offset := t.Zone()
	offsetHours := offset / 3600
	tzSign := "+"
	if offsetHours < 0 {
		tzSign = "-"
		offsetHours = -offsetHours
	}
	return fmt.Sprintf("⌚ %s UTC%s%02d", t.Format("2006/01/02 15:04:05"), tzSign, offsetHours)
}

func HumanFormat(numStr string) string {
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		// Try parsing as raw digit regex if matches
		match, _ := regexp.MatchString("^[0-9]+$", numStr)
		if !match {
			return numStr
		}
		numVal, err := strconv.Atoi(numStr)
		if err != nil {
			return numStr
		}
		num = float64(numVal)
	}

	absVal := num
	if num < 0 {
		absVal = -num
	}

	magnitude := 0
	suffixes := []string{"", "K", "M", "B", "T"}
	for absVal >= 1000 && magnitude < len(suffixes)-1 {
		magnitude++
		absVal /= 1000.0
		num /= 1000.0
	}

	formatted := fmt.Sprintf("%.3g", num)
	// Strip trailing zeros and dot if integer representation
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(formatted, "0")
		formatted = strings.TrimRight(formatted, ".")
	}
	return formatted + suffixes[magnitude]
}

func FormatReactionsStr(likes, cmts, shares string) string {
	var parts []string
	if likes != "" && likes != "null" && likes != "0" {
		parts = append(parts, "❤️ "+likes)
	}
	if cmts != "" && cmts != "null" && cmts != "0" {
		parts = append(parts, "💬 "+cmts)
	}
	if shares != "" && shares != "null" && shares != "0" {
		parts = append(parts, "🔁 "+shares)
	}
	
	joined := strings.Join(parts, " • ")
	return strings.ReplaceAll(joined, ",", ".")
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
}

func WarnWebhook(msg string, fileContent []byte, filename string, embed *DiscordEmbed) {
	go func() {
		wh := globalConfig.NotifierWebhook
		if wh == "" || !strings.HasPrefix(wh, "https://discord.com/api/webhooks/") {
			return
		}

		var payload bytes.Buffer
		writer := multipart.NewWriter(&payload)

		// Message content
		if msg != "" {
			_ = writer.WriteField("content", msg)
		}

		// Embeds payload (if present)
		if embed != nil {
			embedJSON := fmt.Sprintf(`[{"title": %q, "description": %q, "color": %d`, embed.Title, embed.Description, embed.Color)
			if len(embed.Fields) > 0 {
				embedJSON += `, "fields": [`
				for i, field := range embed.Fields {
					if i > 0 {
						embedJSON += ","
					}
					embedJSON += fmt.Sprintf(`{"name": %q, "value": %q, "inline": %t}`, field.Name, field.Value, field.Inline)
				}
				embedJSON += `]`
			}
			embedJSON += `}]`
			_ = writer.WriteField("payload_json", fmt.Sprintf(`{"embeds": %s}`, embedJSON))
		}

		// File attachment
		if len(fileContent) > 0 && filename != "" {
			part, err := writer.CreateFormFile("file", filename)
			if err == nil {
				_, _ = part.Write(fileContent)
			}
		}

		_ = writer.Close()

		req, err := http.NewRequest("POST", wh, &payload)
		if err != nil {
			log.Printf("Failed to create webhook request: %v", err)
			return
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to send webhook: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Webhook returned status %d: %s", resp.StatusCode, string(body))
		}
	}()
}
