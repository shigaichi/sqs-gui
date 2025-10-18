package internal

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/olivere/vite"
	"github.com/shigaichi/sqs-gui"
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
	if err := loadTemplate("queues", filepath.Join("templates", "pages", "queues.gohtml")); err != nil {
		return nil, errors.Wrap(err, "failed to load queues template")
	}
	if err := loadTemplate("queue", filepath.Join("templates", "pages", "queue.gohtml")); err != nil {
		return nil, errors.Wrap(err, "failed to load queue template")
	}
	if err := loadTemplate("create-queue", filepath.Join("templates", "pages", "create-queue.gohtml")); err != nil {
		return nil, errors.Wrap(err, "failed to load create-queue template")
	}
	if err := loadTemplate("send-receive", filepath.Join("templates", "pages", "send-receive.gohtml")); err != nil {
		return nil, errors.Wrap(err, "failed to load send-receive template")
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
			return nil, errors.Wrap(err, "creating sub-filesystem for 'dist' directory")
		}
		viteConfig.FS = distFS

	}

	entries := []string{
		"assets/js/app.ts",
		"assets/js/queues.ts",
		"assets/js/create_queue.ts",
		"assets/js/queue.ts",
		"assets/js/send_receive.ts",
	}

	for _, entry := range entries {
		viteConfig.ViteEntry = entry
		fragment, err := vite.HTMLFragment(viteConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build %s fragment", entry)
		}
		fragments[entry] = fragment
	}

	mux := http.NewServeMux()

	if !isDev {
		// Serve static files from the embedded distribution when not in dev mode.
		// In development Vite serves assets directly, so no handler is required here.
		f := http.FileServer(http.FS(viteConfig.FS))
		mux.Handle("/assets/", f)
		mux.Handle("/icon.svg", f)
	} else {
		assetsDir := http.Dir("assets")
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(assetsDir)))
		mux.Handle("/icon.svg", http.FileServer(http.Dir("public")))
	}

	mux.HandleFunc("/queues", i.h.QueuesHandler)
	mux.HandleFunc("GET /create-queue", i.h.GetCreateQueueHandler)
	mux.HandleFunc("POST /create-queue", i.h.PostCreateQueueHandler)
	mux.HandleFunc("POST /queues/{url}/purge", i.h.PurgeQueueHandler)
	mux.HandleFunc("POST /queues/{url}/delete", i.h.DeleteQueueHandler)
	mux.HandleFunc("/queues/{url}", i.h.QueueHandler)
	mux.HandleFunc("/queues/{url}/send-receive", i.h.SendReceive)
	mux.HandleFunc("POST /queues/{url}/messages", i.h.SendMessageAPI)
	mux.HandleFunc("POST /queues/{url}/messages/poll", i.h.ReceiveMessagesAPI)
	mux.HandleFunc("POST /queues/{url}/messages/delete", i.h.DeleteMessageAPI)

	return logMiddleware(mux), nil
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request completed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func loadTemplate(tmplName string, filename ...string) error {
	base := template.New("layout")
	layoutFiles := []string{
		filepath.Join("templates", "layout.gohtml"),
		filepath.Join("templates", "partials", "head.gohtml"),
		filepath.Join("templates", "partials", "header.gohtml"),
		filepath.Join("templates", "partials", "footer.gohtml"),
	}

	tmpl, err := base.ParseFiles(layoutFiles...)
	if err != nil {
		return errors.Wrap(err, "failed to parse layout")
	}

	tmpl, err = tmpl.ParseFiles(filename...)
	if err != nil {
		return errors.Wrap(err, "failed to parse page template")
	}

	templates[tmplName] = tmpl
	return nil
}
