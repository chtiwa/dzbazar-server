package utils

import (
	"net/http"
	"os"
)

// CrossSiteCookies: admin and API on different *.up.railway.app hosts are
// cross-site (public suffix list), so Lax cookies are dropped and every
// request 401s. Set COOKIE_CROSS_SITE=true there; leave unset for localhost.
func CrossSiteCookies() bool { return os.Getenv("COOKIE_CROSS_SITE") == "true" }

// SecureCookies: Secure flag. SameSite=None requires it.
func SecureCookies() bool { return os.Getenv("APP_ENV") == "production" || CrossSiteCookies() }

func CookieSameSite() http.SameSite {
	if CrossSiteCookies() {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}
