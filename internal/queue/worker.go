package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"c0de-webhook/internal/mail"
	"c0de-webhook/internal/store"
	"c0de-webhook/internal/telegram"
	"c0de-webhook/internal/whatsapp"
)

// ChannelSender provides Send methods for each channel type.
// Nil senders are allowed — messages for unconfigured channels will fail gracefully.
type ChannelSender struct {
	Mail     mail.Sender
	WhatsApp *whatsapp.BusinessSender
	Telegram *telegram.Sender
}

type Worker struct {
	store      *store.Store
	senders    ChannelSender
	sendersMu  sync.RWMutex
	workers    int
	retryDelay time.Duration
	ch         chan store.Message
	wg         sync.WaitGroup
}

// UpdateSenders hot-swaps the channel senders without restarting the worker.
func (w *Worker) UpdateSenders(s ChannelSender) {
	w.sendersMu.Lock()
	w.senders = s
	w.sendersMu.Unlock()
	log.Println("Channel senders reloaded")
}

func NewWorker(st *store.Store, senders ChannelSender, workers int, retryDelay time.Duration) *Worker {
	return &Worker{
		store:      st,
		senders:    senders,
		workers:    workers,
		retryDelay: retryDelay,
		ch:         make(chan store.Message, workers*2),
	}
}

func (w *Worker) Start(ctx context.Context) {
	if err := w.store.ResetStuckMessages(); err != nil {
		log.Printf("WARNING: failed to reset stuck messages: %v", err)
	}

	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.process(ctx, i)
	}

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

		log.Printf("Worker %d: [%s] sending message %d to %s", id, msg.Channel, msg.ID, msg.To)

		err := w.sendByChannel(msg)
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

func (w *Worker) sendByChannel(msg store.Message) error {
	w.sendersMu.RLock()
	senders := w.senders
	w.sendersMu.RUnlock()

	switch msg.Channel {
	case "whatsapp":
		if senders.WhatsApp == nil || !senders.WhatsApp.Configured() {
			return fmt.Errorf("whatsapp sender not configured")
		}
		return senders.WhatsApp.Send(msg.To, msg.TextBody)
	case "telegram":
		if senders.Telegram == nil || !senders.Telegram.Configured() {
			return fmt.Errorf("telegram sender not configured")
		}
		return senders.Telegram.Send(msg.To, msg.TextBody)
	case "mail", "":
		if senders.Mail == nil {
			return fmt.Errorf("mail sender not configured")
		}
		return senders.Mail.Send(msg.To, msg.Subject, msg.TextBody, msg.HTMLBody)
	default:
		return fmt.Errorf("unknown channel: %s", msg.Channel)
	}
}
