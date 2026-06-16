package main

import (
	"fmt"
	"html"
	"strings"
)

func FormatErrorMessageEmbed(originalURL string) string {
	escapedURL := html.EscapeString(originalURL)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="">
<head>
    <meta charset="UTF-8" />
    <meta name="theme-color" content="#2c3048f" />
    <meta property="og:title" content="Log in or sign up to view"/>
    <meta property="og:description" content="See posts, photos and more on Facebook."/>
    <meta http-equiv="refresh" content="0;url=%s"/>
</head>
</html>`, escapedURL)
}

func FormatRedirectPage(urlStr string) string {
	escapedURL := html.EscapeString(urlStr)
	return fmt.Sprintf(`<!DOCTYPE HTML>
<html lang="en-US">
    <head>
        <meta charset="UTF-8">
        <meta http-equiv="refresh" content="0; url=%s">
        <script type="text/javascript">
            window.location.href = "%s"
        </script>
        <title>redirecting...</title>
    </head>
    <body>
    </body>
</html>`, escapedURL, escapedURL)
}

func FormatReelPostEmbed(post ParsedPost, textOnly bool) string {
	var videoMetaTags string
	if !textOnly && len(post.VideoLinks) > 0 {
		var sb strings.Builder
		for _, link := range post.VideoLinks {
			escapedLink := html.EscapeString(link)
			sb.WriteString(fmt.Sprintf("<meta property=\"twitter:player:stream\" content=\"%s\"/>\n", escapedLink))
			sb.WriteString(fmt.Sprintf("<meta property=\"og:video\" content=\"%s\"/>\n", escapedLink))
			sb.WriteString(fmt.Sprintf("<meta property=\"og:video:secure_url\" content=\"%s\"/>\n", escapedLink))
		}
		videoMetaTags = sb.String()
	}

	reactionStr := FormatReactionsStr(post.Likes, post.Comments, post.Shares)
	postDate := TimestampToStr(post.Date)
	color := "#0866ff"

	escapedTitle := html.EscapeString(post.AuthorName)
	
	// Limit description length as in Python post.Text[:1024]
	descText := post.Text
	if len(descText) > 1024 {
		descText = descText[:1024]
	}
	escapedDesc := html.EscapeString(descText)
	escapedCredit := html.EscapeString(GetCredit())
	escapedReaction := html.EscapeString(reactionStr)
	escapedDate := html.EscapeString(postDate)
	escapedURL := html.EscapeString(post.URL)

	siteName := fmt.Sprintf("%s\n%s\n%s", escapedCredit, escapedDate, escapedReaction)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="">
<head>
    <title>%s</title>
    <meta charset="UTF-8"/>
    <meta property="og:title" content="%s"/>
    <meta property="og:description" content="%s"/>
    <meta property="og:site_name" content="%s"/>
    <meta property="og:url" content="%s"/>
    <meta property="og:video:type" content="video/mp4"/>
    <meta property="twitter:player:stream:content_type" content="video/mp4"/>

    %s
    <link rel="canonical" href="%s"/>
    <meta http-equiv="refresh" content="0;url=%s"/>
    <meta name="twitter:card" content="player"/>
    <meta name="theme-color" content="%s"/>
</head>
</html>`, escapedCredit, escapedTitle, escapedDesc, siteName, escapedURL, videoMetaTags, escapedURL, escapedURL, color)
}

func FormatFullPostEmbed(post ParsedPost, textOnly bool) string {
	if len(post.VideoLinks) > 0 {
		return FormatReelPostEmbed(post, textOnly)
	}

	var imageMetaTags string
	var imageCounter string
	if !textOnly && len(post.ImageLinks) > 0 {
		if len(post.ImageLinks) > 4 {
			imageCounter = "\ncontains 4+ images"
		}
		
		limit := 4
		if len(post.ImageLinks) < limit {
			limit = len(post.ImageLinks)
		}
		
		var sb strings.Builder
		for i := 0; i < limit; i++ {
			sb.WriteString(fmt.Sprintf("<meta property=\"og:image\" content=\"%s\"/>\n", html.EscapeString(post.ImageLinks[i])))
		}
		imageMetaTags = sb.String()
	}

	postDate := TimestampToStr(post.Date)
	reactionStr := FormatReactionsStr(post.Likes, post.Comments, post.Shares)

	escapedTitle := html.EscapeString(post.AuthorName)
	descText := post.Text
	if len(descText) > 1024 {
		descText = descText[:1024]
	}
	escapedDesc := html.EscapeString(descText)
	escapedCredit := html.EscapeString(GetCredit())
	escapedReaction := html.EscapeString(reactionStr)
	escapedDate := html.EscapeString(postDate)
	escapedURL := html.EscapeString(post.URL)

	siteName := fmt.Sprintf("%s\n%s\n%s%s", escapedCredit, escapedDate, escapedReaction, imageCounter)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="">
<head>
    <title>%s</title>
    <meta charset="UTF-8"/>
    <meta property="og:title" content="%s"/>
    <meta property="og:description" content="%s"/>
    <meta property="og:site_name" content="%s"/>
    <meta property="og:url" content="%s"/>
    %s
    <link rel="canonical" href="%s"/>
    <meta http-equiv="refresh" content="0;url=%s"/>
    <meta name="twitter:card" content="summary_large_image"/>
    <meta name="theme-color" content="#0866ff"/>
</head>
</html>`, escapedCredit, escapedTitle, escapedDesc, siteName, escapedURL, imageMetaTags, escapedURL, escapedURL)
}
