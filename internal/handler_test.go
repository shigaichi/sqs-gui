package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/olivere/vite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHandlerImpl_QueuesHandler_Success(t *testing.T) {
	queueTime := time.Date(2024, time.May, 1, 15, 4, 5, 0, time.UTC)
	newQueueSummaries := func() []QueueSummary {
		return []QueueSummary{
			{
				URL:                       "https://sqs.local/000000000000/orders",
				Name:                      "orders",
				Type:                      QueueTypeStandard,
				MessagesAvailable:         10,
				MessagesInFlight:          2,
				Encryption:                "SSE",
				ContentBasedDeduplication: false,
			},
			{
				URL:                       "https://sqs.local/000000000000/events.fifo",
				Name:                      "events.fifo",
				Type:                      QueueTypeFIFO,
				CreatedAt:                 queueTime,
				MessagesAvailable:         4,
				MessagesInFlight:          1,
				Encryption:                "SSE",
				ContentBasedDeduplication: true,
			},
		}
	}

	testCases := []struct {
		name       string
		requestURL string
		wantFlash  *pageFlash
	}{
		{
			name:       "without flash message",
			requestURL: "/queues",
			wantFlash:  nil,
		},
		{
			name:       "with created flash message",
			requestURL: "/queues?created=%20orders%20",
			wantFlash: &pageFlash{
				Kind:    "success",
				Message: `Queue "orders" was created successfully.`,
			},
		},
		{
			name:       "with deleted flash message",
			requestURL: "/queues?deleted=%20events.fifo%20",
			wantFlash: &pageFlash{
				Kind:    "success",
				Message: `Queue "events.fifo" was deleted successfully.`,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			req := httptest.NewRequest(http.MethodGet, tc.requestURL, nil)
			queues := newQueueSummaries()

			mockService.EXPECT().
				Queues(mock.MatchedBy(func(ctx context.Context) bool {
					return ctx == req.Context()
				})).
				Return(queues, nil).
				Once()

			handler := NewHandler(mockService)

			var captured queuesPageData
			captureQueuesTemplate(t, &captured)
			installQueuesFragment(t, template.HTML(`<script data-test="queues"></script>`))

			rr := httptest.NewRecorder()
			handler.QueuesHandler(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))

			assert.Equal(t, "Queues", captured.Title)
			assert.Equal(t, template.HTML(`<script data-test="queues"></script>`), captured.ViteTags)
			assert.Empty(t, captured.ErrorMessage)

			if tc.wantFlash == nil {
				assert.Nil(t, captured.Flash)
			} else if assert.NotNil(t, captured.Flash) {
				assert.Equal(t, tc.wantFlash.Kind, captured.Flash.Kind)
				assert.Equal(t, tc.wantFlash.Message, captured.Flash.Message)
			}

			if assert.Len(t, captured.Queues, len(queues)) {
				first := captured.Queues[0]
				assert.Equal(t, "orders", first.Name)
				assert.Equal(t, url.QueryEscape(queues[0].URL), first.URL)
				assert.Equal(t, "STANDARD", first.Type)
				assert.Equal(t, "-", first.CreatedAt)
				assert.Equal(t, "10", first.MessagesAvailable)
				assert.Equal(t, "2", first.MessagesInFlight)
				assert.Equal(t, "SSE", first.Encryption)
				assert.Equal(t, "Disabled", first.ContentBasedDeduplication)

				second := captured.Queues[1]
				assert.Equal(t, "events.fifo", second.Name)
				assert.Equal(t, url.QueryEscape(queues[1].URL), second.URL)
				assert.Equal(t, "FIFO", second.Type)
				assert.Equal(t, queueTime.Format("2006-01-02 15:04:05 MST"), second.CreatedAt)
				assert.Equal(t, "4", second.MessagesAvailable)
				assert.Equal(t, "1", second.MessagesInFlight)
				assert.Equal(t, "SSE", second.Encryption)
				assert.Equal(t, "Enabled", second.ContentBasedDeduplication)
			}
		})
	}
}

func TestHandlerImpl_QueuesHandler_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	req := httptest.NewRequest(http.MethodGet, "/queues", nil)
	mockService.EXPECT().
		Queues(mock.MatchedBy(func(ctx context.Context) bool {
			return ctx == req.Context()
		})).
		Return(nil, errors.New("boom")).
		Once()

	rr := httptest.NewRecorder()
	handler.QueuesHandler(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "failed to load queues\n", rr.Body.String())
}

func TestHandlerImpl_GetCreateQueueHandler(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	var captured createQueuePageData
	captureCreateQueueTemplate(t, &captured)
	installCreateQueueFragment(t, template.HTML(`<script data-test="create"></script>`))

	req := httptest.NewRequest(http.MethodGet, "/create-queue", nil)
	rr := httptest.NewRecorder()
	handler.GetCreateQueueHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))

	assert.Equal(t, "Create Queue", captured.Title)
	assert.Equal(t, template.HTML(`<script data-test="create"></script>`), captured.ViteTags)
	assert.Empty(t, captured.ErrorMessage)
	assert.Equal(t, createQueueForm{Type: string(QueueTypeStandard)}, captured.Form)
	if assert.Len(t, captured.QueueTypes, 2) {
		assert.Equal(t, queueTypeOption{Value: string(QueueTypeStandard), Label: "Standard"}, captured.QueueTypes[0])
		assert.Equal(t, queueTypeOption{Value: string(QueueTypeFIFO), Label: "FIFO"}, captured.QueueTypes[1])
	}
}

func TestHandlerImpl_PostCreateQueueHandler_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	form := url.Values{}
	form.Set("queue_name", "orders")
	form.Set("queue_type", string(QueueTypeFIFO))
	form.Set("delay_seconds", "10")
	form.Set("message_retention_period", "1200")
	form.Set("visibility_timeout", "30")
	form.Set("content_deduplication", "on")

	req := httptest.NewRequest(http.MethodPost, "/create-queue", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		CreateQueue(
			mock.MatchedBy(func(ctx context.Context) bool {
				return ctx == req.Context()
			}),
			mock.MatchedBy(func(input CreateQueueInput) bool {
				if !assert.Equal(t, "orders", input.Name) {
					return false
				}
				if !assert.Equal(t, QueueTypeFIFO, input.Type) {
					return false
				}
				if !assert.NotNil(t, input.DelaySeconds) || !assert.Equal(t, int32(10), *input.DelaySeconds) {
					return false
				}
				if !assert.NotNil(t, input.MessageRetentionPeriod) || !assert.Equal(t, int32(1200), *input.MessageRetentionPeriod) {
					return false
				}
				if !assert.NotNil(t, input.VisibilityTimeout) || !assert.Equal(t, int32(30), *input.VisibilityTimeout) {
					return false
				}
				return assert.True(t, input.ContentBasedDeduplication)
			}),
		).
		Return(CreateQueueResult{QueueURL: "https://sqs.local/000000000000/orders"}, nil).
		Once()

	handler.PostCreateQueueHandler(rr, req)

	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/queues?created=orders", rr.Header().Get("Location"))
}

func TestHandlerImpl_PostCreateQueueHandler_ParseFormError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	req := httptest.NewRequest(http.MethodPost, "/create-queue", strings.NewReader("queue_name=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.PostCreateQueueHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "invalid form\n", rr.Body.String())
}

func TestHandlerImpl_PostCreateQueueHandler_InvalidDelay(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	form := url.Values{}
	form.Set("queue_name", "orders")
	form.Set("queue_type", string(QueueTypeStandard))
	form.Set("delay_seconds", "901")

	req := httptest.NewRequest(http.MethodPost, "/create-queue", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	var captured createQueuePageData
	captureCreateQueueTemplate(t, &captured)
	installCreateQueueFragment(t, template.HTML(`<script data-test="create"></script>`))

	handler.PostCreateQueueHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "Delay seconds must be between 0 and 900.", captured.ErrorMessage)
	assert.Equal(t, "orders", captured.Form.Name)
	assert.Equal(t, string(QueueTypeStandard), captured.Form.Type)
	assert.Equal(t, "901", captured.Form.DelaySeconds)
}

func TestHandlerImpl_PostCreateQueueHandler_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	form := url.Values{}
	form.Set("queue_name", "events")
	form.Set("queue_type", string(QueueTypeStandard))

	req := httptest.NewRequest(http.MethodPost, "/create-queue", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	var captured createQueuePageData
	captureCreateQueueTemplate(t, &captured)
	installCreateQueueFragment(t, template.HTML(`<script data-test="create"></script>`))

	mockService.EXPECT().
		CreateQueue(
			mock.MatchedBy(func(ctx context.Context) bool {
				return ctx == req.Context()
			}),
			mock.Anything,
		).
		Return(CreateQueueResult{}, errors.New("boom")).
		Once()

	handler.PostCreateQueueHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "boom", captured.ErrorMessage)
	assert.Equal(t, "events", captured.Form.Name)
}

func TestHandlerImpl_QueueHandler_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/000000000000/orders.fifo"
	req := httptest.NewRequest(http.MethodGet, "/queues/"+url.QueryEscape(queueURL)+"?purged=1", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	createdAt := time.Date(2024, time.May, 1, 10, 0, 0, 0, time.UTC)
	modifiedAt := time.Date(2024, time.May, 2, 11, 30, 0, 0, time.UTC)
	queueDetail := QueueDetail{
		QueueSummary: QueueSummary{
			URL:                       queueURL,
			Name:                      "orders.fifo",
			Type:                      QueueTypeFIFO,
			CreatedAt:                 createdAt,
			MessagesAvailable:         12,
			MessagesInFlight:          5,
			Encryption:                "SSE",
			ContentBasedDeduplication: true,
		},
		Arn:            "arn:aws:sqs:us-east-1:000000000000:orders.fifo",
		LastModifiedAt: modifiedAt,
		Attributes: map[string]string{
			"VisibilityTimeout": "30",
			"DelaySeconds":      "10",
		},
		Tags: map[string]string{
			"env":  "prod",
			"team": "payments",
		},
	}

	mockService.EXPECT().
		QueueDetail(
			mock.MatchedBy(func(ctx context.Context) bool {
				return ctx == req.Context()
			}),
			mock.MatchedBy(func(s string) bool {
				return assert.Equal(t, queueURL, s)
			}),
		).
		Return(queueDetail, nil).
		Once()

	var captured queuePageData
	captureQueueTemplate(t, &captured)
	installQueueFragment(t, template.HTML(`<script data-test="queue"></script>`))

	handler.QueueHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "Queue orders.fifo", captured.Title)
	assert.Equal(t, template.HTML(`<script data-test="queue"></script>`), captured.ViteTags)
	assert.Equal(t, `All messages in "orders.fifo" were purged successfully.`, captured.FlashMessage)
	assert.Equal(t, queueDetail.URL, captured.Queue.URL)
	assert.Equal(t, url.QueryEscape(queueURL), captured.Queue.EscapedURL)
	assert.Equal(t, "FIFO", captured.Queue.Type)
	assert.Equal(t, createdAt.Format("2006-01-02 15:04:05 MST"), captured.Queue.CreatedAt)
	assert.Equal(t, modifiedAt.Format("2006-01-02 15:04:05 MST"), captured.Queue.LastModifiedAt)
	assert.Equal(t, "12", captured.Queue.MessagesAvailable)
	assert.Equal(t, "5", captured.Queue.MessagesInFlight)
	assert.Equal(t, "Enabled", captured.Queue.ContentBasedDeduplication)
	if assert.Len(t, captured.Queue.Attributes, 2) {
		assert.Equal(t, queueAttributeView{Key: "DelaySeconds", Value: "10"}, captured.Queue.Attributes[0])
		assert.Equal(t, queueAttributeView{Key: "VisibilityTimeout", Value: "30"}, captured.Queue.Attributes[1])
	}
	if assert.Len(t, captured.Queue.Tags, 2) {
		assert.Equal(t, queueTagView{Key: "env", Value: "prod"}, captured.Queue.Tags[0])
		assert.Equal(t, queueTagView{Key: "team", Value: "payments"}, captured.Queue.Tags[1])
	}
}

func TestHandlerImpl_QueueHandler_BadQueueURL(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func(req *http.Request)
		expectBody string
	}{
		{
			name:       "missing path value",
			setup:      func(_ *http.Request) {},
			expectBody: "queue url is required\n",
		},
		{
			name: "invalid encoding",
			setup: func(req *http.Request) {
				req.SetPathValue("url", "%%%")
			},
			expectBody: "invalid queue url\n",
		},
		{
			name: "blank after decode",
			setup: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("   "))
			},
			expectBody: "queue url is required\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			req := httptest.NewRequest(http.MethodGet, "/queues/{url}", nil)
			rr := httptest.NewRecorder()
			tc.setup(req)

			handler.QueueHandler(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, tc.expectBody, rr.Body.String())
		})
	}
}

func TestHandlerImpl_QueueHandler_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodGet, "/queues/"+url.QueryEscape(queueURL), nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		QueueDetail(mock.Anything, queueURL).
		Return(QueueDetail{}, errors.New("boom")).
		Once()

	handler.QueueHandler(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, "failed to load queue detail\n", rr.Body.String())
}

func TestHandlerImpl_DeleteQueueHandler_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/delete", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		DeleteQueue(mock.Anything, queueURL).
		Return(nil).
		Once()

	handler.DeleteQueueHandler(rr, req)

	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/queues?deleted=orders", rr.Header().Get("Location"))
}

func TestHandlerImpl_DeleteQueueHandler_BadQueueURL(t *testing.T) {
	testCases := []struct {
		name       string
		set        func(req *http.Request)
		expectBody string
	}{
		{
			name:       "missing",
			set:        func(_ *http.Request) {},
			expectBody: "queue url is required\n",
		},
		{
			name: "invalid",
			set: func(req *http.Request) {
				req.SetPathValue("url", "%")
			},
			expectBody: "invalid queue url\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/queues/{url}/delete", nil)
			rr := httptest.NewRecorder()
			tc.set(req)

			handler.DeleteQueueHandler(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, tc.expectBody, rr.Body.String())
		})
	}
}

func TestHandlerImpl_DeleteQueueHandler_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/delete", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		DeleteQueue(mock.Anything, queueURL).
		Return(errors.New("boom")).
		Once()

	handler.DeleteQueueHandler(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, "failed to delete queue\n", rr.Body.String())
}

func TestHandlerImpl_PurgeQueueHandler_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/purge", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		PurgeQueue(mock.Anything, queueURL).
		Return(nil).
		Once()

	handler.PurgeQueueHandler(rr, req)

	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/queues/"+url.QueryEscape(queueURL)+"?purged=1", rr.Header().Get("Location"))
}

func TestHandlerImpl_PurgeQueueHandler_BadQueueURL(t *testing.T) {
	testCases := []struct {
		name       string
		set        func(req *http.Request)
		expectBody string
	}{
		{
			name:       "missing",
			set:        func(_ *http.Request) {},
			expectBody: "queue url is required\n",
		},
		{
			name: "invalid",
			set: func(req *http.Request) {
				req.SetPathValue("url", "%")
			},
			expectBody: "invalid queue url\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/queues/{url}/purge", nil)
			rr := httptest.NewRecorder()
			tc.set(req)

			handler.PurgeQueueHandler(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, tc.expectBody, rr.Body.String())
		})
	}
}

func TestHandlerImpl_PurgeQueueHandler_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/purge", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		PurgeQueue(mock.Anything, queueURL).
		Return(errors.New("boom")).
		Once()

	handler.PurgeQueueHandler(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, "failed to purge queue\n", rr.Body.String())
}

func TestHandlerImpl_SendReceive_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/events.fifo"
	req := httptest.NewRequest(http.MethodGet, "/queues/"+url.QueryEscape(queueURL)+"/send-receive", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	detail := QueueDetail{
		QueueSummary: QueueSummary{
			URL:  queueURL,
			Name: "events.fifo",
			Type: QueueTypeFIFO,
		},
	}

	mockService.EXPECT().
		QueueDetail(mock.Anything, queueURL).
		Return(detail, nil).
		Once()

	var captured sendReceivePageData
	captureSendReceiveTemplate(t, &captured)
	installSendReceiveFragment(t, template.HTML(`<script data-test="send-receive"></script>`))

	handler.SendReceive(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "Send and receive messages · events.fifo", captured.Title)
	assert.Equal(t, template.HTML(`<script data-test="send-receive"></script>`), captured.ViteTags)
	assert.Equal(t, detail.Name, captured.Queue.Name)
	assert.Equal(t, detail.URL, captured.Queue.URL)
	assert.Equal(t, url.QueryEscape(queueURL), captured.Queue.EscapedURL)
	assert.Equal(t, "FIFO", captured.Queue.Type)
	assert.True(t, captured.Queue.SupportsMessageGroups)
}

func TestHandlerImpl_SendReceive_BadQueueURL(t *testing.T) {
	testCases := []struct {
		name       string
		set        func(req *http.Request)
		expectBody string
	}{
		{
			name:       "missing",
			set:        func(_ *http.Request) {},
			expectBody: "queue url is required\n",
		},
		{
			name: "invalid",
			set: func(req *http.Request) {
				req.SetPathValue("url", "%")
			},
			expectBody: "invalid queue url\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			req := httptest.NewRequest(http.MethodGet, "/queues/{url}/send-receive", nil)
			rr := httptest.NewRecorder()
			tc.set(req)

			handler.SendReceive(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, tc.expectBody, rr.Body.String())
		})
	}
}

func TestHandlerImpl_SendReceive_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/events"
	req := httptest.NewRequest(http.MethodGet, "/queues/{url}/send-receive", nil)
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		QueueDetail(mock.Anything, queueURL).
		Return(QueueDetail{}, errors.New("boom")).
		Once()

	handler.SendReceive(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, "failed to load queue detail\n", rr.Body.String())
}

func TestHandlerImpl_SendMessageAPI_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	payload := sendMessageRequest{
		Body:           "hello",
		MessageGroupID: " group ",
		DelaySeconds:   ptrInt32(5),
		Attributes: []messageAttributePayload{
			{Name: " id ", Value: "123"},
			{Name: "", Value: "ignored"},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages", bytes.NewReader(body))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		SendMessage(
			mock.Anything,
			mock.MatchedBy(func(input SendMessageInput) bool {
				if !assert.Equal(t, queueURL, input.QueueURL) {
					return false
				}
				if !assert.Equal(t, "hello", input.Body) {
					return false
				}
				if !assert.Equal(t, " group ", input.MessageGroupID) {
					return false
				}
				if !assert.NotNil(t, input.DelaySeconds) || !assert.Equal(t, int32(5), *input.DelaySeconds) {
					return false
				}
				if !assert.Equal(t, []MessageAttribute{{Name: "id", Value: "123"}}, input.Attributes) {
					return false
				}
				return true
			}),
		).
		Return(nil).
		Once()

	handler.SendMessageAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "{\"message\":\"Message sent successfully.\"}\n", rr.Body.String())
}

func TestHandlerImpl_SendMessageAPI_BadRequests(t *testing.T) {
	testCases := []struct {
		name       string
		setRequest func(req *http.Request)
		body       []byte
		expect     string
	}{
		{
			name:       "missing queue url",
			setRequest: func(_ *http.Request) {},
			body:       []byte(`{"body":"hello"}`),
			expect:     "{\"error\":\"queue url is required\"}\n",
		},
		{
			name: "invalid queue url",
			setRequest: func(req *http.Request) {
				req.SetPathValue("url", "%")
			},
			body:   []byte(`{"body":"hello"}`),
			expect: "{\"error\":\"invalid queue url\"}\n",
		},
		{
			name: "request body required",
			setRequest: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("https://sqs.local/queues/orders"))
			},
			body:   nil,
			expect: "{\"error\":\"request body is required\"}\n",
		},
		{
			name: "invalid json",
			setRequest: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("https://sqs.local/queues/orders"))
			},
			body:   []byte(`{"body":`),
			expect: "{\"error\":\"invalid request body\"}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			var bodyReader *bytes.Reader
			if tc.body == nil {
				bodyReader = bytes.NewReader([]byte{})
			} else {
				bodyReader = bytes.NewReader(tc.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages", bodyReader)
			rr := httptest.NewRecorder()
			tc.setRequest(req)

			handler.SendMessageAPI(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
			assert.Equal(t, tc.expect, rr.Body.String())
		})
	}
}

func TestHandlerImpl_SendMessageAPI_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages", bytes.NewReader([]byte(`{"body":"hi"}`)))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		SendMessage(mock.Anything, mock.Anything).
		Return(errors.New("boom")).
		Once()

	handler.SendMessageAPI(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "{\"error\":\"boom\"}\n", rr.Body.String())
}

func TestHandlerImpl_ReceiveMessagesAPI_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	payload := receiveMessagesRequest{MaxMessages: ptrInt32(5), WaitTimeSeconds: ptrInt32(15)}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/poll", bytes.NewReader(body))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	result := ReceiveMessagesResult{
		Messages: []ReceivedMessage{
			{
				ID:            "id-1",
				Body:          "hello",
				ReceiptHandle: "rh",
				ReceiveCount:  2,
				Attributes: []MessageAttribute{
					{Name: "key", Value: "value"},
				},
			},
		},
	}

	mockService.EXPECT().
		ReceiveMessages(
			mock.MatchedBy(func(ctx context.Context) bool { return ctx == req.Context() }),
			mock.MatchedBy(func(input ReceiveMessagesInput) bool {
				if !assert.Equal(t, queueURL, input.QueueURL) {
					return false
				}
				return assert.Equal(t, ReceiveMessagesInput{
					QueueURL:            queueURL,
					MaxMessages:         5,
					WaitTimeSeconds:     15,
					MaxMessagesProvided: true,
					WaitTimeProvided:    true,
				}, input)
			}),
		).
		Return(result, nil).
		Once()

	handler.ReceiveMessagesAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

	var response receiveMessagesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if assert.Len(t, response.Messages, 1) {
		msg := response.Messages[0]
		assert.Equal(t, "id-1", msg.ID)
		assert.Equal(t, "hello", msg.Body)
		assert.Equal(t, "rh", msg.ReceiptHandle)
		assert.Equal(t, int32(2), msg.ReceiveCount)
		assert.Equal(t, []messageAttributeResponse{{Name: "key", Value: "value"}}, msg.Attributes)
	}
}

func TestHandlerImpl_ReceiveMessagesAPI_Defaults(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/poll", bytes.NewReader(nil))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		ReceiveMessages(
			mock.Anything,
			mock.MatchedBy(func(input ReceiveMessagesInput) bool {
				return assert.Equal(t, ReceiveMessagesInput{QueueURL: queueURL}, input)
			}),
		).
		Return(ReceiveMessagesResult{}, nil).
		Once()

	handler.ReceiveMessagesAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandlerImpl_ReceiveMessagesAPI_BadRequests(t *testing.T) {
	testCases := []struct {
		name       string
		set        func(req *http.Request)
		body       []byte
		expectBody string
	}{
		{
			name:       "missing queue url",
			set:        func(_ *http.Request) {},
			body:       []byte(`{}`),
			expectBody: "{\"error\":\"queue url is required\"}\n",
		},
		{
			name: "invalid queue url",
			set: func(req *http.Request) {
				req.SetPathValue("url", "%")
			},
			body:       []byte(`{}`),
			expectBody: "{\"error\":\"invalid queue url\"}\n",
		},
		{
			name: "invalid json",
			set: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("https://sqs.local/queues/orders"))
			},
			body:       []byte(`{"maxMessages":true}`),
			expectBody: "{\"error\":\"invalid request body\"}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/poll", bytes.NewReader(tc.body))
			rr := httptest.NewRecorder()
			tc.set(req)

			handler.ReceiveMessagesAPI(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Equal(t, tc.expectBody, rr.Body.String())
		})
	}
}

func TestHandlerImpl_ReceiveMessagesAPI_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/poll", bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		ReceiveMessages(mock.Anything, mock.Anything).
		Return(ReceiveMessagesResult{}, errors.New("boom")).
		Once()

	handler.ReceiveMessagesAPI(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "{\"error\":\"boom\"}\n", rr.Body.String())
}

func TestHandlerImpl_DeleteMessageAPI_Success(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/delete", bytes.NewReader([]byte(`{"receiptHandle":"abc"}`)))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		DeleteMessage(
			mock.Anything,
			mock.MatchedBy(func(input DeleteMessageInput) bool {
				return assert.Equal(t, DeleteMessageInput{QueueURL: queueURL, ReceiptHandle: "abc"}, input)
			}),
		).
		Return(nil).
		Once()

	handler.DeleteMessageAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "{\"message\":\"Message deleted successfully.\"}\n", rr.Body.String())
}

func TestHandlerImpl_DeleteMessageAPI_BadRequests(t *testing.T) {
	testCases := []struct {
		name   string
		set    func(req *http.Request)
		body   []byte
		code   int
		expect string
	}{
		{
			name:   "missing queue url",
			set:    func(_ *http.Request) {},
			body:   []byte(`{"receiptHandle":"abc"}`),
			code:   http.StatusBadRequest,
			expect: "{\"error\":\"queue url is required\"}\n",
		},
		{
			name: "invalid queue url",
			set: func(req *http.Request) {
				req.SetPathValue("url", "%")
			},
			body:   []byte(`{"receiptHandle":"abc"}`),
			code:   http.StatusBadRequest,
			expect: "{\"error\":\"invalid queue url\"}\n",
		},
		{
			name: "request body required",
			set: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("https://sqs.local/queues/orders"))
			},
			body:   []byte{},
			code:   http.StatusBadRequest,
			expect: "{\"error\":\"request body is required\"}\n",
		},
		{
			name: "invalid json",
			set: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("https://sqs.local/queues/orders"))
			},
			body:   []byte(`{"receiptHandle":123}`),
			code:   http.StatusBadRequest,
			expect: "{\"error\":\"invalid request body\"}\n",
		},
		{
			name: "empty receipt handle",
			set: func(req *http.Request) {
				req.SetPathValue("url", url.QueryEscape("https://sqs.local/queues/orders"))
			},
			body:   []byte(`{"receiptHandle":"  "}`),
			code:   http.StatusBadRequest,
			expect: "{\"error\":\"receipt handle is required\"}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := NewMockSqsService(t)
			handler := NewHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/delete", bytes.NewReader(tc.body))
			rr := httptest.NewRecorder()
			tc.set(req)

			handler.DeleteMessageAPI(rr, req)

			assert.Equal(t, tc.code, rr.Code)
			assert.Equal(t, tc.expect, rr.Body.String())
		})
	}
}

func TestHandlerImpl_DeleteMessageAPI_ServiceError(t *testing.T) {
	mockService := NewMockSqsService(t)
	handler := NewHandler(mockService)

	queueURL := "https://sqs.local/queues/orders"
	req := httptest.NewRequest(http.MethodPost, "/queues/{url}/messages/delete", bytes.NewReader([]byte(`{"receiptHandle":"abc"}`)))
	req.SetPathValue("url", url.QueryEscape(queueURL))
	rr := httptest.NewRecorder()

	mockService.EXPECT().
		DeleteMessage(mock.Anything, mock.Anything).
		Return(errors.New("boom")).
		Once()

	handler.DeleteMessageAPI(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "{\"error\":\"boom\"}\n", rr.Body.String())
}

func captureQueuesTemplate(t *testing.T, captured *queuesPageData) {
	t.Helper()
	captureTemplate(t, "queues", func(data queuesPageData) { *captured = data })
}

func installQueuesFragment(t *testing.T, tags template.HTML) {
	t.Helper()
	installFragment(t, "assets/js/queues.ts", tags)
}

func captureCreateQueueTemplate(t *testing.T, captured *createQueuePageData) {
	t.Helper()
	captureTemplate(t, "create-queue", func(data createQueuePageData) { *captured = data })
}

func installCreateQueueFragment(t *testing.T, tags template.HTML) {
	t.Helper()
	installFragment(t, "assets/js/create_queue.ts", tags)
}

func captureQueueTemplate(t *testing.T, captured *queuePageData) {
	t.Helper()
	captureTemplate(t, "queue", func(data queuePageData) { *captured = data })
}

func installQueueFragment(t *testing.T, tags template.HTML) {
	t.Helper()
	installFragment(t, "assets/js/queue.ts", tags)
}

func captureSendReceiveTemplate(t *testing.T, captured *sendReceivePageData) {
	t.Helper()
	captureTemplate(t, "send-receive", func(data sendReceivePageData) { *captured = data })
}

func installSendReceiveFragment(t *testing.T, tags template.HTML) {
	t.Helper()
	installFragment(t, "assets/js/send_receive.ts", tags)
}

func captureTemplate[T any](t *testing.T, name string, assign func(T)) {
	t.Helper()

	tmpl := template.Must(template.New(name).Funcs(template.FuncMap{
		"capture": func(data T) string {
			assign(data)
			return ""
		},
	}).Parse(`{{capture .}}`))

	prev, ok := templates[name]
	templates[name] = tmpl

	t.Cleanup(func() {
		if ok {
			templates[name] = prev
		} else {
			delete(templates, name)
		}
	})
}

func installFragment(t *testing.T, entry string, tags template.HTML) {
	t.Helper()

	prev, ok := fragments[entry]
	fragments[entry] = &vite.Fragment{Tags: tags}

	t.Cleanup(func() {
		if ok {
			fragments[entry] = prev
		} else {
			delete(fragments, entry)
		}
	})
}

func ptrInt32(v int32) *int32 {
	return &v
}
