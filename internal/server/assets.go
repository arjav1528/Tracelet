package server

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	webassets "github.com/arjav1528/Tracelet/web"
)

func serveAsset(w http.ResponseWriter, request *http.Request) {
	name := strings.TrimPrefix(request.URL.Path, "/")
	if name == "" {
		name = "index.html"
	} else if !strings.HasPrefix(name, "assets/") {
		http.NotFound(w, request)
		return
	}
	if len(name) > 2048 || !fs.ValidPath(name) || request.URL.RawPath != "" {
		http.NotFound(w, request)
		return
	}
	data, err := fs.ReadFile(webassets.FS(), name)
	if err != nil {
		http.NotFound(w, request)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "asset requests require GET or HEAD", http.StatusMethodNotAllowed)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, request, name, time.Time{}, bytes.NewReader(data))
}
