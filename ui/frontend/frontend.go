package frontend

import (
	"embed"
	"io/fs"
	"net/http"
)

var (
	//go:generate pnpm install
	//go:generate pnpm build
	//go:embed all:build
	staticFS embed.FS

	fileSystem fs.FS
)

func init() {
	var err error
	fileSystem, err = fs.Sub(staticFS, "build")
	if err != nil {
		panic(err)
	}
}

func FileSystem() fs.FS {
	return fileSystem
}

func FileServer() http.Handler {
	return http.FileServer(http.FS(fileSystem))
}
