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
	DeleteQueueHandler(w http.ResponseWriter, r *http.Request)
	PurgeQueueHandler(w http.ResponseWriter, r *http.Request)
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

type pageFlash struct {
	Message string
	Kind    string
}

type queuesPageData struct {
	Title        string
	Queues       []queueView
	ViteTags     template.HTML
	Flash        *pageFlash
	ErrorMessage string
}

type queuePageData struct {
	Title        string
	Queue        queueDetailView
	ViteTags     template.HTML
	FlashMessage string
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

	var flash *pageFlash
	query := r.URL.Query()
	if created := strings.TrimSpace(query.Get("created")); created != "" {
		flash = &pageFlash{
			Message: fmt.Sprintf("Queue \"%s\" was created successfully.", created),
			Kind:    "success",
		}
	} else if deleted := strings.TrimSpace(query.Get("deleted")); deleted != "" {
		flash = &pageFlash{
			Message: fmt.Sprintf("Queue \"%s\" was deleted successfully.", deleted),
			Kind:    "success",
		}
	}

	data := queuesPageData{
		Title:    "Queues",
		Queues:   viewQueues,
		ViteTags: fragments["assets/js/queues.ts"].Tags,
		Flash:    flash,
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
	queueURL, status, err := h.queueURLFromRequest(r)
	if err != nil {
		if status == 0 {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
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
			EscapedURL:                url.QueryEscape(queueURL),
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

	if r.URL.Query().Get("purged") == "1" {
		data.FlashMessage = fmt.Sprintf("All messages in \"%s\" were purged successfully.", queueDetail.Name)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := templates["queue"].Execute(w, data); err != nil {
		slog.Error("failed to render queue template", slog.Any("error", err))
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// DeleteQueueHandler handles POST requests to delete a queue entirely.
func (h *HandlerImpl) DeleteQueueHandler(w http.ResponseWriter, r *http.Request) {
	queueURL, status, err := h.queueURLFromRequest(r)
	if err != nil {
		if status == 0 {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	if err := h.s.DeleteQueue(r.Context(), queueURL); err != nil {
		slog.Error("failed to delete queue", slog.String("queue_url", queueURL), slog.Any("error", err))
		http.Error(w, "failed to delete queue", http.StatusInternalServerError)
		return
	}

	queueName := extractQueueName(queueURL)
	redirectURL := fmt.Sprintf("/queues?deleted=%s", url.QueryEscape(queueName))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// PurgeQueueHandler handles POST requests to purge all messages in a queue.
func (h *HandlerImpl) PurgeQueueHandler(w http.ResponseWriter, r *http.Request) {
	queueURL, status, err := h.queueURLFromRequest(r)
	if err != nil {
		if status == 0 {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	if err := h.s.PurgeQueue(r.Context(), queueURL); err != nil {
		slog.Error("failed to purge queue", slog.String("queue_url", queueURL), slog.Any("error", err))
		http.Error(w, "failed to purge queue", http.StatusInternalServerError)
		return
	}

	redirectURL := fmt.Sprintf("/queues/%s?purged=1", url.QueryEscape(queueURL))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *HandlerImpl) queueURLFromRequest(r *http.Request) (string, int, error) {
	encodedURL := r.PathValue("url")
	if encodedURL == "" {
		return "", http.StatusBadRequest, fmt.Errorf("queue url is required")
	}

	queueURL, err := url.QueryUnescape(encodedURL)
	if err != nil {
		return "", http.StatusBadRequest, fmt.Errorf("invalid queue url")
	}

	if strings.TrimSpace(queueURL) == "" {
		return "", http.StatusBadRequest, fmt.Errorf("queue url is required")
	}

	return queueURL, 0, nil
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
