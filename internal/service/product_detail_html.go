package service

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var allowedProductDetailTags = map[string]struct{}{
	"a": {}, "blockquote": {}, "br": {}, "caption": {}, "code": {}, "div": {},
	"em": {}, "figcaption": {}, "figure": {}, "h1": {}, "h2": {}, "h3": {},
	"h4": {}, "h5": {}, "h6": {}, "hr": {}, "img": {}, "li": {}, "ol": {},
	"p": {}, "pre": {}, "span": {}, "strong": {}, "table": {}, "tbody": {},
	"td": {}, "tfoot": {}, "th": {}, "thead": {}, "tr": {}, "u": {}, "ul": {},
}

var blockedProductDetailTags = map[string]struct{}{
	"embed": {}, "form": {}, "iframe": {}, "input": {}, "link": {}, "meta": {},
	"noscript": {}, "object": {}, "script": {}, "style": {}, "svg": {}, "textarea": {},
}

func sanitizeProductDetailHTML(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	contextNode := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(raw), contextNode)
	if err != nil {
		return ""
	}

	root := &html.Node{Type: html.ElementNode, Data: "div"}
	for _, node := range nodes {
		appendSanitizedProductDetailNode(root, node)
	}

	var output strings.Builder
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&output, child); err != nil {
			return ""
		}
	}
	return strings.TrimSpace(output.String())
}

func appendSanitizedProductDetailNode(parent *html.Node, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: node.Data})
	case html.ElementNode:
		tag := strings.ToLower(node.Data)
		if _, blocked := blockedProductDetailTags[tag]; blocked {
			return
		}
		if _, allowed := allowedProductDetailTags[tag]; !allowed {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				appendSanitizedProductDetailNode(parent, child)
			}
			return
		}

		clean := &html.Node{Type: html.ElementNode, Data: tag}
		for _, attr := range node.Attr {
			if sanitized, ok := sanitizeProductDetailAttribute(tag, attr); ok {
				clean.Attr = append(clean.Attr, sanitized)
			}
		}
		parent.AppendChild(clean)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			appendSanitizedProductDetailNode(clean, child)
		}
	}
}

func sanitizeProductDetailAttribute(tag string, attr html.Attribute) (html.Attribute, bool) {
	key := strings.ToLower(strings.TrimSpace(attr.Key))
	value := strings.TrimSpace(attr.Val)
	if key == "" || strings.HasPrefix(key, "on") {
		return html.Attribute{}, false
	}

	switch key {
	case "alt", "title":
		if tag == "img" || tag == "a" {
			return html.Attribute{Key: key, Val: value}, true
		}
	case "colspan", "rowspan":
		if tag == "td" || tag == "th" {
			return html.Attribute{Key: key, Val: value}, true
		}
	case "href":
		if tag == "a" && isAllowedProductDetailLink(value) {
			return html.Attribute{Key: key, Val: value}, true
		}
	case "src":
		if tag == "img" && isAllowedProductDetailImage(value) {
			return html.Attribute{Key: key, Val: value}, true
		}
	}
	return html.Attribute{}, false
}

func isAllowedProductDetailImage(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/uploads/") {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) && parsed.Host != ""
}

func isAllowedProductDetailLink(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "#") {
		return !strings.HasPrefix(value, "//")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "mailto", "tel":
		return parsed.Opaque != "" || parsed.Path != ""
	default:
		return false
	}
}
