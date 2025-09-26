package internal

import (
	"html/template"
	"net/http"
	"time"
)

type Handler interface {
	QueuesHandler(w http.ResponseWriter, r *http.Request)
	CreateQueueHandler(w http.ResponseWriter, r *http.Request)
}

type HandlerImpl struct {
	s SqsService
}

func NewHandler(s SqsService) *HandlerImpl {
	return &HandlerImpl{s: s}
}

type queuesPageData struct {
	Title    string
	Now      string
	ViteTags template.HTML
}

func (h *HandlerImpl) QueuesHandler(w http.ResponseWriter, r *http.Request) {
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst).Format("2006-01-02 15:04:05 MST")

	// TODO: サービスクラス呼び出し例
	h.s.Queues()

	data := queuesPageData{
		Title: "Queues",
		// FIXME: サンプルとしてNowを渡しているが、実際は渡す必要はない。各ページごとに必要な情報を渡す
		Now:      now,
		ViteTags: fragments["assets/js/queues.ts"].Tags,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := templates["queues"].Execute(w, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

type createQueuePageData struct {
	Title    string
	Now      string
	ViteTags template.HTML
}

func (h *HandlerImpl) CreateQueueHandler(w http.ResponseWriter, r *http.Request) {
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst).Format("2006-01-02 15:04:05 MST")

	// TODO: サービスクラス呼び出し例
	h.s.CreateQueue()

	data := createQueuePageData{
		Title: "Create Queue",
		// FIXME: サンプルとしてNowを渡しているが、実際は渡す必要はない。各ページごとに必要な情報を渡す
		Now:      now,
		ViteTags: fragments["assets/js/create_queue.ts"].Tags,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := templates["create-queue"].Execute(w, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}
