package queue

import (
	"context"
	"log"
	"sync"
	"time"

	"c0de-webhook/internal/mail"
	"c0de-webhook/internal/store"
)

type Worker struct {
	store      *store.Store
	sender     mail.Sender
	workers    int
	retryDelay time.Duration
	ch         chan store.Message
	wg         sync.WaitGroup
}

func NewWorker(st *store.Store, sender mail.Sender, workers int, retryDelay time.Duration) *Worker {
	return &Worker{
		store:      st,
		sender:     sender,
		workers:    workers,
		retryDelay: retryDelay,
		ch:         make(chan store.Message, workers*2),
	}
}

func (w *Worker) Start(ctx context.Context) {
	// Reset any messages stuck in "sending" from a previous crash
	if err := w.store.ResetStuckMessages(); err != nil {
		log.Printf("WARNING: failed to reset stuck messages: %v", err)
	}

	// Start worker goroutines
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.process(ctx, i)
	}

	// Start dispatcher
	w.wg.Add(1)
	go w.dispatch(ctx)

	log.Printf("Queue started with %d workers", w.workers)
}

func (w *Worker) Stop() {
	w.wg.Wait()
	log.Println("Queue workers stopped")
}

func (w *Worker) dispatch(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.ch)
			return
		case <-ticker.C:
			messages, err := w.store.ClaimPendingMessages(w.workers)
			if err != nil {
				log.Printf("ERROR claiming messages: %v", err)
				continue
			}
			for _, msg := range messages {
				select {
				case w.ch <- msg:
				case <-ctx.Done():
					close(w.ch)
					return
				}
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, id int) {
	defer w.wg.Done()

	for msg := range w.ch {
		select {
		case <-ctx.Done():
			return
		default:
		}

		log.Printf("Worker %d: sending message %d to %s", id, msg.ID, msg.To)

		err := w.sender.Send(msg.To, msg.Subject, msg.TextBody, msg.HTMLBody)
		if err != nil {
			log.Printf("Worker %d: message %d failed: %v", id, msg.ID, err)
			if markErr := w.store.MarkFailed(msg.ID, err.Error(), w.retryDelay); markErr != nil {
				log.Printf("Worker %d: failed to mark message %d as failed: %v", id, msg.ID, markErr)
			}
		} else {
			log.Printf("Worker %d: message %d sent successfully", id, msg.ID)
			if markErr := w.store.MarkSent(msg.ID); markErr != nil {
				log.Printf("Worker %d: failed to mark message %d as sent: %v", id, msg.ID, markErr)
			}
		}
	}
}
