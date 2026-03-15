package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3" // CGO SQLite driver required by whatsmeow
)

// WebClient wraps whatsmeow for sending WhatsApp messages via the Web protocol.
// Free alternative to the Business API — requires scanning a QR code once.
type WebClient struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.RWMutex
	qrPNG     []byte // latest QR code PNG for the UI
	qrDone    bool   // true when QR flow is complete
	linking   bool   // true when actively waiting for QR scan
}

// NewWebClient creates a WhatsApp Web client with SQLite session storage.
// dbPath should be a file path like "./data/whatsapp.db".
func NewWebClient(dbPath string) (*WebClient, error) {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening whatsapp db: %w", err)
	}

	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	if err := container.Upgrade(context.Background()); err != nil {
		return nil, fmt.Errorf("upgrading whatsapp store: %w", err)
	}

	wc := &WebClient{container: container}

	// Try to restore existing session
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	wc.client = whatsmeow.NewClient(deviceStore, nil)

	// If already logged in, connect automatically
	if wc.client.Store.ID != nil {
		if err := wc.client.Connect(); err != nil {
			log.Printf("WARNING: WhatsApp Web auto-connect failed: %v", err)
		} else {
			log.Println("WhatsApp Web: connected (restored session)")
		}
	}

	return wc, nil
}

// IsConnected returns true if the client has an active connection.
func (wc *WebClient) IsConnected() bool {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.client != nil && wc.client.IsConnected()
}

// IsLoggedIn returns true if the client has a stored session.
func (wc *WebClient) IsLoggedIn() bool {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.client != nil && wc.client.Store.ID != nil
}

// IsLinking returns true if QR code linking is in progress.
func (wc *WebClient) IsLinking() bool {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.linking
}

// GetQRPNG returns the latest QR code PNG image, or nil if not linking.
func (wc *WebClient) GetQRPNG() []byte {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.qrPNG
}

// StartLinking begins the QR code pairing process.
// Call GetQRPNG() to get the QR image, and poll Status() to check progress.
func (wc *WebClient) StartLinking() error {
	wc.mu.Lock()
	if wc.linking {
		wc.mu.Unlock()
		return nil // already linking
	}
	wc.linking = true
	wc.qrDone = false
	wc.qrPNG = nil
	wc.mu.Unlock()

	// Disconnect existing client if any
	if wc.client.IsConnected() {
		wc.client.Disconnect()
	}

	// Get a fresh device store
	deviceStore, err := wc.container.GetFirstDevice(context.Background())
	if err != nil {
		wc.mu.Lock()
		wc.linking = false
		wc.mu.Unlock()
		return fmt.Errorf("getting device: %w", err)
	}
	wc.client = whatsmeow.NewClient(deviceStore, nil)

	qrChan, _ := wc.client.GetQRChannel(context.Background())
	if err := wc.client.Connect(); err != nil {
		wc.mu.Lock()
		wc.linking = false
		wc.mu.Unlock()
		return fmt.Errorf("connecting: %w", err)
	}

	// Handle QR events in background
	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				png, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
				if err != nil {
					log.Printf("WhatsApp Web: QR encode error: %v", err)
					continue
				}
				wc.mu.Lock()
				wc.qrPNG = png
				wc.mu.Unlock()
				log.Println("WhatsApp Web: QR code generated, waiting for scan...")
			case "success":
				log.Println("WhatsApp Web: paired successfully!")
				wc.mu.Lock()
				wc.qrPNG = nil
				wc.qrDone = true
				wc.linking = false
				wc.mu.Unlock()
				return
			case "timeout":
				log.Println("WhatsApp Web: QR timeout")
				wc.mu.Lock()
				wc.qrPNG = nil
				wc.linking = false
				wc.mu.Unlock()
				return
			}
		}
	}()

	return nil
}

// Disconnect disconnects and removes the stored session.
func (wc *WebClient) Disconnect() {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	if wc.client != nil {
		_ = wc.client.Logout(context.Background())
		wc.linking = false
		wc.qrPNG = nil
	}
}

// Send sends a text message to the given phone number.
// Phone should be in international format without + (e.g. "41791234567").
func (wc *WebClient) Send(phone, text string) error {
	if !wc.IsConnected() {
		return fmt.Errorf("whatsapp web not connected")
	}

	jid := types.NewJID(phone, types.DefaultUserServer)
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := wc.client.SendMessage(ctx, jid, msg)
	if err != nil {
		return fmt.Errorf("sending whatsapp message: %w", err)
	}

	return nil
}

// Status returns the current connection status as a string.
func (wc *WebClient) Status() string {
	if wc.IsLinking() {
		return "linking"
	}
	if wc.IsConnected() {
		return "connected"
	}
	if wc.IsLoggedIn() {
		return "disconnected"
	}
	return "not configured"
}

// Configured returns true if there's a stored session (even if not currently connected).
func (wc *WebClient) Configured() bool {
	return wc.IsLoggedIn()
}
