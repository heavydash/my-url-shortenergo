// audit/example_test.go
package audit

import (
	"fmt"
	"time"
)

// Пример создания события сокращения URL.
func ExampleNewShortenEvent() {
	userID := "550e8400-e29b-41d4-a716-446655440000"
	originalURL := "https://example.com/very/long/path"

	event := NewShortenEvent(userID, originalURL)

	fmt.Printf("Event Action: %s\n", event.Action)
	fmt.Printf("Event UserID: %s\n", event.UserID)
	fmt.Printf("Event URL: %s\n", event.URL)
	fmt.Printf("Has Timestamp: %v\n", event.Timestamp > 0)

	// Output:
	// Event Action: shorten
	// Event UserID: 550e8400-e29b-41d4-a716-446655440000
	// Event URL: https://example.com/very/long/path
	// Has Timestamp: true
}

// Пример создания события перехода по ссылке.
func ExampleNewFollowEvent() {
	userID := "user-123"
	originalURL := "https://go.dev/doc"

	event := NewFollowEvent(userID, originalURL)

	fmt.Printf("Event Type: %s\n", event.Action)
	fmt.Printf("For URL: %s\n", event.URL)
	fmt.Printf("Timestamp recent: %v\n",
		time.Now().Unix()-event.Timestamp < 5) // Within 5 seconds

	// Output:
	// Event Type: follow
	// For URL: https://go.dev/doc
	// Timestamp recent: true
}

// Пример сериализации события в JSON.
func ExampleEvent_serialization() {
	event := &Event{
		Timestamp: 1705314600,
		Action:    "shorten",
		UserID:    "user-123",
		URL:       "https://example.com",
	}

	// В реальном коде используйте json.Marshal
	jsonStr := fmt.Sprintf(
		`{"ts":%d,"action":"%s","user_id":"%s","url":"%s"}`,
		event.Timestamp, event.Action, event.UserID, event.URL,
	)

	fmt.Println("JSON representation:")
	fmt.Println(jsonStr)

	// Output:
	// JSON representation:
	// {"ts":1705314600,"action":"shorten","user_id":"user-123","url":"https://example.com"}
}
