package internal

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Handler defines the HTTP handlers exposed by the service.
type Handler interface {
	QueuesHandler(w http.ResponseWriter, r *http.Request)
	GetCreateQueueHandler(w http.ResponseWriter, r *http.Request)
	PostCreateQueueHandler(w http.ResponseWriter, r *http.Request)
	QueueHandler(w http.ResponseWriter, r *http.Request)
}

// HandlerImpl implements the HTTP handlers.
type HandlerImpl struct {
	s SqsService
}

// NewHandler creates a new HandlerImpl instance.
func NewHandler(s SqsService) *HandlerImpl {
	return &HandlerImpl{s: s}
}

type queueView struct {
	Name                      string
	URL                       string
	Type                      string
	CreatedAt                 string
	MessagesAvailable         string
	MessagesInFlight          string
	Encryption                string
	ContentBasedDeduplication string
}

type queuesPageData struct {
	Title        string
	Queues       []queueView
	ViteTags     template.HTML
	FlashMessage string
	ErrorMessage string
}

type queuePageData struct {
	Title    string
	Queue    queueDetailView
	ViteTags template.HTML
}

type queueDetailView struct {
	Name                      string
	URL                       string
	EscapedURL                string
	Arn                       string
	Type                      string
	CreatedAt                 string
	LastModifiedAt            string
	MessagesAvailable         string
	MessagesInFlight          string
	Encryption                string
	ContentBasedDeduplication string
	Attributes                []queueAttributeView
	Tags                      []queueTagView
}

type queueAttributeView struct {
	Key   string
	Value string
}

type queueTagView struct {
	Key   string
	Value string
}

type queueTypeOption struct {
	Value string
	Label string
}

type createQueueForm struct {
	Name                   string
	Type                   string
	DelaySeconds           string
	MessageRetentionPeriod string
	VisibilityTimeout      string
	ContentBasedDedup      bool
}

type createQueuePageData struct {
	Title        string
	ViteTags     template.HTML
	Form         createQueueForm
	QueueTypes   []queueTypeOption
	ErrorMessage string
}

// QueuesHandler renders the queue listing page.
func (h *HandlerImpl) QueuesHandler(w http.ResponseWriter, r *http.Request) {
	queues, err := h.s.Queues(r.Context())
	if err != nil {
		slog.Error("failed to load queue list", slog.Any("error", err))
		http.Error(w, "failed to load queues", http.StatusInternalServerError)
		return
	}

	viewQueues := make([]queueView, 0, len(queues))
	for _, queue := range queues {
		created := "-"
		if !queue.CreatedAt.IsZero() {
			created = queue.CreatedAt.Format("2006-01-02 15:04:05 MST")
		}

		viewQueues = append(viewQueues, queueView{
			Name:                      queue.Name,
			URL:                       url.QueryEscape(queue.URL),
			Type:                      strings.ToUpper(string(queue.Type)),
			CreatedAt:                 created,
			MessagesAvailable:         strconv.FormatInt(queue.MessagesAvailable, 10),
			MessagesInFlight:          strconv.FormatInt(queue.MessagesInFlight, 10),
			Encryption:                queue.Encryption,
			ContentBasedDeduplication: boolLabel(queue.ContentBasedDeduplication),
		})
	}

	flash := r.URL.Query().Get("created")

	data := queuesPageData{
		Title:        "Queues",
		Queues:       viewQueues,
		ViteTags:     fragments["assets/js/queues.ts"].Tags,
		FlashMessage: flash,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := templates["queues"].Execute(w, data); err != nil {
		slog.Error("failed to render queue template", slog.Any("error", err))
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// GetCreateQueueHandler serves the queue creation page.
func (h *HandlerImpl) GetCreateQueueHandler(w http.ResponseWriter, _ *http.Request) {
	h.renderCreateQueue(w, createQueuePageData{
		Title:      "Create Queue",
		ViteTags:   fragments["assets/js/create_queue.ts"].Tags,
		Form:       h.defaultCreateQueueForm(),
		QueueTypes: queueTypeOptions(),
	})
}

// PostCreateQueueHandler handles POST submissions.
func (h *HandlerImpl) PostCreateQueueHandler(w http.ResponseWriter, r *http.Request) {
	h.handleCreateQueuePost(w, r)
}

func (h *HandlerImpl) handleCreateQueuePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	form := createQueueForm{
		Name:                   strings.TrimSpace(r.FormValue("queue_name")),
		Type:                   r.FormValue("queue_type"),
		DelaySeconds:           strings.TrimSpace(r.FormValue("delay_seconds")),
		MessageRetentionPeriod: strings.TrimSpace(r.FormValue("message_retention_period")),
		VisibilityTimeout:      strings.TrimSpace(r.FormValue("visibility_timeout")),
		ContentBasedDedup:      r.FormValue("content_deduplication") == "on",
	}

	input := CreateQueueInput{
		Name:                      form.Name,
		Type:                      QueueType(form.Type),
		ContentBasedDeduplication: form.ContentBasedDedup,
	}

	var err error
	if input.DelaySeconds, err = parseOptionalInt32(form.DelaySeconds, 0, 900, "Delay seconds must be between 0 and 900."); err != nil {
		h.renderCreateQueue(w, h.createQueueErrorData(form, err))
		return
	}
	if input.MessageRetentionPeriod, err = parseOptionalInt32(form.MessageRetentionPeriod, 60, 1209600, "Message retention period must be between 60 and 1209600."); err != nil {
		h.renderCreateQueue(w, h.createQueueErrorData(form, err))
		return
	}
	if input.VisibilityTimeout, err = parseOptionalInt32(form.VisibilityTimeout, 0, 43200, "Visibility timeout must be between 0 and 43200."); err != nil {
		h.renderCreateQueue(w, h.createQueueErrorData(form, err))
		return
	}

	result, err := h.s.CreateQueue(r.Context(), input)
	if err != nil {
		slog.Error("failed to create queue", slog.Any("error", err))
		h.renderCreateQueue(w, h.createQueueErrorData(form, err))
		return
	}

	createdName := extractQueueName(result.QueueURL)
	redirectURL := fmt.Sprintf("/queues?created=%s", url.QueryEscape(createdName))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *HandlerImpl) renderCreateQueue(w http.ResponseWriter, data createQueuePageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates["create-queue"].Execute(w, data); err != nil {
		slog.Error("failed to render create-queue template", slog.Any("error", err))
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *HandlerImpl) defaultCreateQueueForm() createQueueForm {
	return createQueueForm{Type: string(QueueTypeStandard)}
}

func (h *HandlerImpl) createQueueErrorData(form createQueueForm, err error) createQueuePageData {
	return createQueuePageData{
		Title:        "Create Queue",
		ViteTags:     fragments["assets/js/create_queue.ts"].Tags,
		Form:         form,
		QueueTypes:   queueTypeOptions(),
		ErrorMessage: err.Error(),
	}
}

func (h *HandlerImpl) QueueHandler(w http.ResponseWriter, r *http.Request) {
	encodedURL := r.PathValue("url")
	queueURL, err := url.QueryUnescape(encodedURL)
	if err != nil {
		http.Error(w, "invalid queue url", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(queueURL) == "" {
		http.Error(w, "queue url is required", http.StatusBadRequest)
		return
	}

	queueDetail, err := h.s.QueueDetail(r.Context(), queueURL)
	if err != nil {
		slog.Error("failed to load queue detail", slog.String("queue_url", queueURL), slog.Any("error", err))
		http.Error(w, "failed to load queue detail", http.StatusInternalServerError)
		return
	}

	attributes := make([]queueAttributeView, 0, len(queueDetail.Attributes))
	for key, value := range queueDetail.Attributes {
		attributes = append(attributes, queueAttributeView{
			Key:   key,
			Value: value,
		})
	}
	sort.Slice(attributes, func(i, j int) bool {
		return attributes[i].Key < attributes[j].Key
	})

	tags := make([]queueTagView, 0, len(queueDetail.Tags))
	for key, value := range queueDetail.Tags {
		tags = append(tags, queueTagView{Key: key, Value: value})
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Key < tags[j].Key
	})

	createdAt := "-"
	if !queueDetail.CreatedAt.IsZero() {
		createdAt = queueDetail.CreatedAt.Format("2006-01-02 15:04:05 MST")
	}

	lastModified := "-"
	if !queueDetail.LastModifiedAt.IsZero() {
		lastModified = queueDetail.LastModifiedAt.Format("2006-01-02 15:04:05 MST")
	}

	data := queuePageData{
		Title: fmt.Sprintf("Queue %s", queueDetail.Name),
		Queue: queueDetailView{
			Name:                      queueDetail.Name,
			URL:                       queueDetail.URL,
			EscapedURL:                encodedURL,
			Arn:                       queueDetail.Arn,
			Type:                      strings.ToUpper(string(queueDetail.Type)),
			CreatedAt:                 createdAt,
			LastModifiedAt:            lastModified,
			MessagesAvailable:         strconv.FormatInt(queueDetail.MessagesAvailable, 10),
			MessagesInFlight:          strconv.FormatInt(queueDetail.MessagesInFlight, 10),
			Encryption:                queueDetail.Encryption,
			ContentBasedDeduplication: boolLabel(queueDetail.ContentBasedDeduplication),
			Attributes:                attributes,
			Tags:                      tags,
		},
		ViteTags: fragments["assets/js/queue.ts"].Tags,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := templates["queue"].Execute(w, data); err != nil {
		slog.Error("failed to render queue template", slog.Any("error", err))
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func queueTypeOptions() []queueTypeOption {
	return []queueTypeOption{
		{Value: string(QueueTypeStandard), Label: "Standard"},
		{Value: string(QueueTypeFIFO), Label: "FIFO"},
	}
}

func parseOptionalInt32(raw string, min, max int32, message string) (*int32, error) {
	if raw == "" {
		return nil, nil
	}

	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("%s", message)
	}

	if value < int64(min) || value > int64(max) {
		return nil, fmt.Errorf("%s", message)
	}

	converted := int32(value)
	return &converted, nil
}

func boolLabel(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func extractQueueName(queueURL string) string {
	if idx := strings.LastIndex(queueURL, "/"); idx >= 0 {
		return queueURL[idx+1:]
	}
	return queueURL
}
