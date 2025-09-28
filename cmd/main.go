package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/cockroachdb/errors"
	"github.com/shigaichi/sqs-gui/internal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	sqsClient, err := newSQSClient(ctx)
	if err != nil {
		slog.Error("failed to initialize SQS client", slog.Any("error", err))
		os.Exit(1)
	}

	repo := internal.NewSqsRepository(sqsClient)
	service := internal.NewSqsService(repo)
	handler := internal.NewHandler(service)

	routerImpl := internal.NewRouteImpl(handler)
	router, err := routerImpl.InitRoute()
	if err != nil {
		slog.Error("failed to initialize router", slog.Any("error", err))
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 3 * time.Minute,
		ReadTimeout:       1 * time.Minute,
		WriteTimeout:      1 * time.Minute,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("received SIGINT; shutting down server")
	case err := <-serverErrCh:
		if errors.Is(err, http.ErrServerClosed) {
			slog.Info("server shut down gracefully")
		} else if err != nil {
			slog.Error("failed to start server", slog.Any("error", err))
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("failed to shut down server", slog.Any("error", err))
	}

	slog.Info("server stopped")
}

func newSQSClient(ctx context.Context) (*sqs.Client, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	endpoint := os.Getenv("AWS_SQS_ENDPOINT")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))

	if err != nil {
		return nil, errors.Wrap(err, "failed to load AWS configuration")
	}

	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return client, nil
}
