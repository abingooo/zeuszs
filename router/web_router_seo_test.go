package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexableWebPathLimitsSearchResultsToPublicBrandPages(t *testing.T) {
	tests := []struct {
		path      string
		indexable bool
	}{
		{path: "/", indexable: true},
		{path: "/about", indexable: true},
		{path: "/about/", indexable: true},
		{path: "/sign-in", indexable: false},
		{path: "/dashboard", indexable: false},
		{path: "/system-settings", indexable: false},
		{path: "/organization", indexable: false},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.indexable, isIndexableWebPath(test.path))
		})
	}
}

func TestIndexPageForPathUsesAboutCanonicalURL(t *testing.T) {
	indexPage := []byte(`
<title>宙斯智算 ZEUSZS | New API</title>
<meta name="title" content="宙斯智算 ZEUSZS | New API" />
<!-- ZEUSZS_PUBLIC_META_START -->
宙斯智算 ZEUSZS 为企业研发团队和科研课题组提供统一的 AI 模型接入、成员协作、用量管理与成本控制平台。
<link rel="canonical" href="https://zeuszs.ai/" />
<meta property="og:url" content="https://zeuszs.ai/" />
<!-- ZEUSZS_PUBLIC_META_END -->
<!-- ZEUSZS_SEO_SHELL_START -->
<main data-seo-shell>首页内容</main>
<!-- ZEUSZS_SEO_SHELL_END -->`)

	aboutPage := string(indexPageForPath(indexPage, "/about/"))
	homePage := string(indexPageForPath(indexPage, "/"))

	assert.Contains(t, aboutPage, `关于宙斯智算 ZEUSZS | New API`)
	assert.Contains(t, aboutPage, string(aboutDescription))
	assert.Contains(t, aboutPage, `href="https://zeuszs.ai/about"`)
	assert.Contains(t, aboutPage, `content="https://zeuszs.ai/about"`)
	assert.Contains(t, aboutPage, `关于宙斯智算</h1>`)
	assert.NotContains(t, aboutPage, `首页内容`)
	assert.NotContains(t, aboutPage, string(rootCanonicalTag))
	assert.Equal(t, string(indexPage), homePage)
}

func TestIndexPageForPathMarksPrivatePagesNoIndex(t *testing.T) {
	indexPage := append([]byte{}, publicMetadataStart...)
	indexPage = append(indexPage, indexRobotsTag...)
	indexPage = append(indexPage, rootCanonicalTag...)
	indexPage = append(indexPage, rootOpenGraphURL...)
	indexPage = append(indexPage, []byte(`<script type="application/ld+json">{}</script>`)...)
	indexPage = append(indexPage, publicMetadataEnd...)
	indexPage = append(indexPage, searchShellStart...)
	indexPage = append(indexPage, []byte(`<main data-seo-shell>首页内容</main>`)...)
	indexPage = append(indexPage, searchShellEnd...)

	privatePage := string(indexPageForPath(indexPage, "/dashboard"))

	assert.Contains(t, privatePage, string(noIndexRobotsTag))
	assert.NotContains(t, privatePage, string(indexRobotsTag))
	assert.NotContains(t, privatePage, string(rootCanonicalTag))
	assert.NotContains(t, privatePage, string(rootOpenGraphURL))
	assert.NotContains(t, privatePage, `application/ld+json`)
	assert.NotContains(t, privatePage, `data-seo-shell`)
}
