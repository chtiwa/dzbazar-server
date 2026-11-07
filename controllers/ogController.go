package controllers

import (
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Link-preview crawlers (Facebook, WhatsApp, TikTok...) don't run JS, so the
// SPA's index.html always previews as "DZ Bazare". The edge (Cloudflare)
// rewrites crawler User-Agents to GET /v1/og/<original path>; we return the
// built client index.html with title/description/og:* injected.

var (
	ogShellMu  sync.Mutex
	ogShell    string
	ogShellAt  time.Time
	titleTagRe = regexp.MustCompile(`(?s)<title>.*?</title>`)
	ogDescRe   = regexp.MustCompile(`<meta[^>]+(?:name="description"|property="og:[^"]*"|name="twitter:[^"]*")[^>]*>`)
)

// ponytail: shell cached 5 min in-process; per-instance, no Redis needed for one small file.
func clientShell() (string, error) {
	ogShellMu.Lock()
	defer ogShellMu.Unlock()
	if ogShell != "" && time.Since(ogShellAt) < 5*time.Minute {
		return ogShell, nil
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(strings.TrimRight(os.Getenv("CLIENT_URL"), "/") + "/")
	if err != nil {
		return ogShell, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return ogShell, err
	}
	ogShell, ogShellAt = string(b), time.Now()
	return ogShell, nil
}

func ogTrunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

type ogMeta struct{ title, desc, image string }

func ogLookup(parts []string) (ogMeta, bool) {
	db := initializers.DB
	var m ogMeta

	productOf := func(slug, id string) bool {
		pid, err := uuid.Parse(id)
		if err != nil {
			return false
		}
		var shop models.Shop
		if db.Select("id", "name", "is_active").Where("slug = ?", slug).First(&shop).Error != nil || !shop.IsActive {
			return false
		}
		var p models.Product
		if db.Preload("Images").Where("id = ? AND shop_id = ? AND active = ? AND hidden_by_platform_at IS NULL", pid, shop.ID, true).First(&p).Error != nil {
			return false
		}
		m = ogMeta{p.Title, p.Description, ""}
		for _, im := range p.Images {
			if m.image == "" || im.OrderIndex < 1 {
				m.image = im.URL
			}
		}
		return true
	}

	landingOf := func(id string) bool {
		lid, err := uuid.Parse(id)
		if err != nil {
			return false
		}
		var lp models.LandingPage
		if db.Preload("Images").Preload("Product").Preload("Shop").Where("id = ? AND active = ?", lid, true).First(&lp).Error != nil || !lp.Shop.IsActive {
			return false
		}
		m = ogMeta{lp.Title, lp.Product.Description, ""}
		if len(lp.Images) > 0 {
			m.image = lp.Images[0].URL
		}
		return true
	}

	switch {
	case len(parts) == 2 && parts[0] == "landing-pages":
		return m, landingOf(parts[1])
	case len(parts) == 1 && parts[0] != "":
		var shop models.Shop
		if db.Preload("LogoImage").Where("slug = ? AND is_active = ?", strings.ToLower(parts[0]), true).First(&shop).Error != nil {
			return m, false
		}
		m = ogMeta{title: shop.Name, desc: shop.Description}
		if shop.LogoImage != nil {
			m.image = shop.LogoImage.URL
		}
		return m, true
	case len(parts) == 3 && parts[1] == "products":
		return m, productOf(parts[0], parts[2])
	case len(parts) == 3 && parts[1] == "landing-pages":
		return m, landingOf(parts[2])
	}
	return m, false
}

func OGPage(c *gin.Context) {
	shell, err := clientShell()
	if shell == "" {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		RespondError(c, http.StatusBadGateway, "Client shell unavailable", err)
		return
	}

	parts := strings.FieldsFunc(c.Param("path"), func(r rune) bool { return r == '/' })
	out := shell
	if m, ok := ogLookup(parts); ok {
		t := html.EscapeString(ogTrunc(m.title, 90))
		d := html.EscapeString(ogTrunc(m.desc, 200))
		tags := `<title>` + t + `</title>` +
			`<meta name="description" content="` + d + `">` +
			`<meta property="og:type" content="website">` +
			`<meta property="og:title" content="` + t + `">` +
			`<meta property="og:description" content="` + d + `">` +
			`<meta name="twitter:card" content="summary_large_image">`
		if m.image != "" {
			tags += `<meta property="og:image" content="` + html.EscapeString(m.image) + `">` +
				`<meta name="twitter:image" content="` + html.EscapeString(m.image) + `">`
		}
		// drop the shell's defaults so crawlers don't see duplicates
		out = titleTagRe.ReplaceAllString(out, "")
		out = ogDescRe.ReplaceAllString(out, "")
		out = strings.Replace(out, "</head>", tags+"</head>", 1)
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(out))
}
