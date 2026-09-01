package router

import (
	"bytes"
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

var (
	rootTitleTag        = []byte(`<title>宙斯智算 ZEUSZS | New API</title>`)
	aboutTitleTag       = []byte(`<title>关于宙斯智算 ZEUSZS | New API</title>`)
	rootMetaTitleTag    = []byte(`<meta name="title" content="宙斯智算 ZEUSZS | New API" />`)
	aboutMetaTitleTag   = []byte(`<meta name="title" content="关于宙斯智算 ZEUSZS | New API" />`)
	rootCanonicalTag    = []byte(`<link rel="canonical" href="https://zeuszs.ai/" />`)
	aboutCanonicalTag   = []byte(`<link rel="canonical" href="https://zeuszs.ai/about" />`)
	rootOpenGraphURL    = []byte(`<meta property="og:url" content="https://zeuszs.ai/" />`)
	aboutOpenGraphURL   = []byte(`<meta property="og:url" content="https://zeuszs.ai/about" />`)
	rootDescription     = []byte(`宙斯智算 ZEUSZS 为企业研发团队和科研课题组提供统一的 AI 模型接入、成员协作、用量管理与成本控制平台。`)
	aboutDescription    = []byte(`了解宙斯智算 ZEUSZS 的平台定位、运营主体与开源项目说明。`)
	indexRobotsTag      = []byte(`<meta name="robots" content="index, follow, max-image-preview:large" />`)
	noIndexRobotsTag    = []byte(`<meta name="robots" content="noindex, nofollow" />`)
	publicMetadataStart = []byte(`<!-- ZEUSZS_PUBLIC_META_START -->`)
	publicMetadataEnd   = []byte(`<!-- ZEUSZS_PUBLIC_META_END -->`)
	searchShellStart    = []byte(`<!-- ZEUSZS_SEO_SHELL_START -->`)
	searchShellEnd      = []byte(`<!-- ZEUSZS_SEO_SHELL_END -->`)
	aboutSearchShell    = []byte(`<main id="seo-shell" data-seo-shell style="box-sizing: border-box; min-height: 100vh; padding: 12vh max(24px, 8vw); color: #111827; background: #f8fafc; font-family: system-ui, sans-serif">
  <div style="max-width: 760px">
    <p style="margin: 0 0 16px; color: #475569; font-size: 16px">ZEUSZS</p>
    <h1 style="margin: 0; font-size: clamp(40px, 8vw, 72px); line-height: 1.05">关于宙斯智算</h1>
    <p style="margin: 28px 0 0; font-size: 20px; line-height: 1.7">宙斯智算为企业研发团队和科研课题组提供统一的 AI 模型接入、组织协作、用量管理与成本控制能力。</p>
    <p style="margin: 20px 0 0; color: #475569; line-height: 1.7">平台由宙斯智算（上海）科技有限公司运营。</p>
    <p style="margin: 28px 0 0"><a href="/" style="color: #1d4ed8">返回宙斯智算首页</a></p>
  </div>
</main>`)
)

func isIndexableWebPath(requestPath string) bool {
	return requestPath == "/" || requestPath == "/about" || requestPath == "/about/"
}

func replaceMarkedSection(page, startMarker, endMarker, replacement []byte) []byte {
	start := bytes.Index(page, startMarker)
	if start < 0 {
		return page
	}
	endOffset := bytes.Index(page[start+len(startMarker):], endMarker)
	if endOffset < 0 {
		return page
	}
	end := start + len(startMarker) + endOffset + len(endMarker)

	replaced := make([]byte, 0, len(page)-end+start+len(replacement))
	replaced = append(replaced, page[:start]...)
	replaced = append(replaced, replacement...)
	return append(replaced, page[end:]...)
}

func indexPageForPath(indexPage []byte, requestPath string) []byte {
	if !isIndexableWebPath(requestPath) {
		privatePage := replaceMarkedSection(indexPage, publicMetadataStart, publicMetadataEnd, noIndexRobotsTag)
		return replaceMarkedSection(privatePage, searchShellStart, searchShellEnd, nil)
	}
	if requestPath == "/" {
		return indexPage
	}

	aboutPage := bytes.Replace(indexPage, rootTitleTag, aboutTitleTag, 1)
	aboutPage = bytes.Replace(aboutPage, rootMetaTitleTag, aboutMetaTitleTag, 1)
	aboutPage = bytes.Replace(aboutPage, rootDescription, aboutDescription, 1)
	aboutPage = bytes.Replace(aboutPage, rootCanonicalTag, aboutCanonicalTag, 1)
	aboutPage = bytes.Replace(aboutPage, rootOpenGraphURL, aboutOpenGraphURL, 1)
	return replaceMarkedSection(aboutPage, searchShellStart, searchShellEnd, aboutSearchShell)
}

func SetWebRouter(router *gin.Engine, assets WebAssets) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", frontendFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		if !isIndexableWebPath(c.Request.URL.Path) {
			c.Header("X-Robots-Tag", "noindex, nofollow")
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPageForPath(assets.IndexPage, c.Request.URL.Path))
	})
}
