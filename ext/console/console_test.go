package console

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConsoleStaticContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files := fstest.MapFS{
		"index.html":              {Data: []byte("CONSOLE_SHELL")},
		"static/js/app.abc123.js": {Data: []byte("window.consoleReady=true")},
		".gitkeep":                {Data: []byte("private-placeholder")},
	}
	cases := []struct {
		name, method, path string
		files              fstest.MapFS
		status             int
		body, cache        string
	}{
		{"index", "GET", "/cmh", files, 200, "CONSOLE_SHELL", "no-store"},
		{"trailing_slash", "GET", "/cmh/", files, 200, "CONSOLE_SHELL", "no-store"},
		{"deep_link", "GET", "/cmh/whoami", files, 200, "CONSOLE_SHELL", "no-store"},
		{"head_page", "HEAD", "/cmh/whoami", files, 200, "", "no-store"},
		{"hashed_js", "GET", "/cmh/static/js/app.abc123.js", files, 200, "window.consoleReady=true", "public, max-age=31536000, immutable"},
		{"head_asset", "HEAD", "/cmh/static/js/app.abc123.js", files, 200, "", "public, max-age=31536000, immutable"},
		{"missing_js", "GET", "/cmh/static/js/missing.js", files, 404, "", "no-store"},
		{"directory", "GET", "/cmh/static/js/", files, 404, "", "no-store"},
		{"hidden_file", "GET", "/cmh/.gitkeep", files, 404, "", "no-store"},
		{"traversal", "GET", "/cmh/../README.md", files, 404, "", "no-store"},
		{"unbuilt_index", "GET", "/cmh", fstest.MapFS{}, 503, "", "no-store"},
		{"unbuilt_head", "HEAD", "/cmh/", fstest.MapFS{}, 503, "", "no-store"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := gin.New()
			mount(e, tc.files)
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.body, w.Body.String())
			require.Equal(t, tc.cache, w.Header().Get("Cache-Control"))
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		})
	}
}
