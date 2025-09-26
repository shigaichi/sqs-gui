package internal

import (
	"github.com/cockroachdb/errors"
	"github.com/olivere/vite"
	"github.com/shigaichi/sqs-gui"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	templates = make(map[string]*template.Template)
	fragments = make(map[string]*vite.Fragment)
)

type Route interface {
	InitRoute() (http.Handler, error)
}

type RouteImpl struct {
	h Handler
}

func NewRouteImpl(h Handler) *RouteImpl {
	return &RouteImpl{h: h}
}

func (i RouteImpl) InitRoute() (http.Handler, error) {
	// FIXME: リファクタリング
	// FIXME: error handling
	err := loadTemplate("queues", filepath.Join("templates", "pages", "queues.gohtml"))
	if err != nil {
		return nil, errors.Wrap(err, "loading templates")
	}
	err = loadTemplate("create-queue", filepath.Join("templates", "pages", "create-queue.gohtml"))
	if err != nil {
		return nil, errors.Wrap(err, "loading templates")
	}

	isDev := os.Getenv("DEV_MODE") == "true"

	viteConfig := vite.Config{
		IsDev:        isDev,
		ViteTemplate: vite.VanillaTs,
	}
	if isDev {
		viteConfig.ViteURL = "http://localhost:5173"
	} else {
		dist := sqs_gui.Dist
		distFS, err := fs.Sub(dist, "dist")
		if err != nil {
			// FIXME: error handling
			return nil, errors.Wrapf(err, "creating sub-filesystem for 'dist' directory: %v", err)
		}
		viteConfig.FS = distFS

	}

	// FIXME: リファクタリング
	viteConfig.ViteEntry = "static/js/app.ts"
	h1, _ := vite.HTMLFragment(viteConfig)
	fragments["assets/js/app.ts"] = h1
	viteConfig.ViteEntry = "assets/js/queues.ts"
	h2, _ := vite.HTMLFragment(viteConfig)
	fragments["assets/js/queues.ts"] = h2
	viteConfig.ViteEntry = "assets/js/create_queue.ts"
	h3, _ := vite.HTMLFragment(viteConfig)
	fragments["assets/js/create_queue.ts"] = h3

	mux := http.NewServeMux()

	if !isDev {
		// 静的ファイル
		// isDevの場合はviteサーバーへアクセスするため設定不要
		f := http.FileServer(http.FS(viteConfig.FS))
		mux.Handle("/assets/", f)
	} else {
		publicDir := http.Dir("assets")
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(publicDir)))
	}

	mux.HandleFunc("/queues", i.h.QueuesHandler)
	mux.HandleFunc("/create-queue", i.h.CreateQueueHandler)

	return logMiddleware(mux), nil
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func loadTemplate(tmplName string, filename ...string) error {
	tmpl, err := template.New("layout").ParseFiles(
		filepath.Join("templates", "layout.gohtml"),
		filepath.Join("templates", "partials", "head.gohtml"),
		filepath.Join("templates", "partials", "header.gohtml"),
		filepath.Join("templates", "partials", "footer.gohtml"),
	)
	if err != nil {
		return errors.Wrap(err, "parse layout failed")
	}

	templates[tmplName] = template.Must(tmpl.ParseFiles(filename...))

	return nil
}
