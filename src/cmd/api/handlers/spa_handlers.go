package handlers

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// EmbeddedFiles хранит скомпилированную статику
var EmbeddedFiles embed.FS

func SPAHandler() http.Handler {
	distFS, err := fs.Sub(EmbeddedFiles, "dist")
	if err != nil {
		log.Fatalf("failed to create sub fs: %v", err)
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(r.URL.Path)
		if path == "." || path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		f, err := distFS.Open(path[1:])
		if err != nil {
			if os.IsNotExist(err) {
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
		} else {
			_ = f.Close()
		}

		fileServer.ServeHTTP(w, r)
	})
}
