package main

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/shigaichi/sqs-gui/internal"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	q := sqs.Client{} // TODO: 適切に生成
	x := internal.NewSqsRepository(q)
	s := internal.NewSqsService(x)
	h := internal.NewHandler(s)
	i := internal.NewRouteImpl(h)

	r, err := i.InitRoute()
	if err != nil {
		// FIXME: error handling
		panic(err)
	}

	srv := http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 3 * time.Minute,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				println("shutting down server")
			} else {
				println("Failed to start server")
			}
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	<-c

	var wait = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	err = srv.Shutdown(ctx)
	if err != nil {
		println("Failed to shutdown server")
	}
	println("Shutting down")
}
