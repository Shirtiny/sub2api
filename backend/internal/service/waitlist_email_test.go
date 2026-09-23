package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestWaitlistConfirmationHTML(t *testing.T) {
	body := waitlistConfirmationHTML("Café Shop")
	require.Equal(t, 1, strings.Count(body, waitlistConfirmationMessage))
	require.Contains(t, body, "好事，正在酝酿。")
	require.Contains(t, body, "已加入 Waiting List")
	require.Contains(t, body, "prefers-color-scheme: dark")
	require.Contains(t, body, "max-width: 480px")
	require.NotContains(t, body, "{{SITE_NAME}}")
	require.NotContains(t, body, "{{MESSAGE}}")
	require.Less(t, len(body), 32*1024)

	// It remains useful without remote images, web fonts, scripts or links.
	doc, err := html.Parse(strings.NewReader(body))
	require.NoError(t, err)
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			require.NotContains(t, []string{"script", "img", "svg", "iframe", "link", "form", "a"}, n.Data)
			for _, attr := range n.Attr {
				require.NotContains(t, []string{"src", "href", "background"}, attr.Key)
			}
			if n.Data == "table" {
				require.Contains(t, n.Attr, html.Attribute{Key: "role", Val: "presentation"})
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	require.NotContains(t, body, "url(")
	require.NotContains(t, body, "@import")
}

func TestWaitlistConfirmationHTMLBranding(t *testing.T) {
	for _, tc := range []struct {
		name, escaped string
	}{
		{"Café Shop", "Café Shop"},
		{"  My Site  ", "My Site"},
		{"", "Sub2API"},
		{" \n\t ", "Sub2API"},
		{`Site & <script>alert("x")</script>`, "Site &amp; &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;"},
		{"{{MESSAGE}}", "{{MESSAGE}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := waitlistConfirmationHTML(tc.name)
			require.Contains(t, body, "Waiting List 申请成功 · "+tc.escaped+"</title>")
			require.Contains(t, body, ">"+tc.escaped+"</p>")
			require.Equal(t, 1, strings.Count(body, waitlistConfirmationMessage))
			require.NotContains(t, body, "<script>")
		})
	}
}

func TestWaitlistConfirmationUsesConfiguredBrand(t *testing.T) {
	mailer := &waitlistMailerStub{}
	svc := NewWaitlistService(&waitlistRepoStub{}, mailer, waitlistBrandingStub("Another & Site"))
	require.NoError(t, svc.Join(context.Background(), "person@example.com"))
	require.Equal(t, waitlistConfirmationHTML("Another & Site"), mailer.body)
	require.NotContains(t, mailer.body, "Café Shop")
}
