package main

import (
	"context"
	"encoding/json"
	"fmt"
	dicedb "github.com/dicedb/go-dice"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
)

var (
	client   *dicedb.Client
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type Score struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

func main() {
	client = dicedb.NewClient(&dicedb.Options{
		Addr: ":7379",
		// Additional options can be set here as needed
	})

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/update", handleUpdate)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	ctx := context.Background()

	// Create a new watch object
	watch := client.WatchCommand(ctx)
	if watch == nil {
		log.Println("Failed to create watch")
		return
	}
	defer watch.Close()

	// Start watching for changes in the leaderboard
	err = watch.Watch(ctx, "ZRANGE", "leaderboard", "0", "5", "REV", "WITHSCORES")
	if err != nil {
		log.Println("Failed to start watch:", err)
		return
	}

	// Get the channel to receive messages
	channel := watch.Channel()

	// Listen for messages and send updates to the client
	for {
		select {
		case msg := <-channel:
			// Parse the message data into []Score
			scores, err := parseScores(msg.Data)
			if err != nil {
				log.Println("Failed to parse scores:", err)
				continue
			}

			// Send the scores to the client
			if err := conn.WriteJSON(scores); err != nil {
				log.Println("WebSocket write error:", err)
				return
			}

		case <-ctx.Done():
			// Client disconnected
			return
		}
	}
}

func parseScores(data interface{}) ([]Score, error) {
	dataList, ok := data.([]interface{})
	if !ok {
		return nil, errUnexpectedDataType
	}

	var scores []Score
	for i := 0; i < len(dataList); i += 2 {
		member, ok1 := dataList[i].(string)
		scoreStr, ok2 := dataList[i+1].(string)
		if !ok1 || !ok2 {
			return nil, errUnexpectedDataType
		}
		scoreFloat, err := strconv.ParseFloat(scoreStr, 64)
		if err != nil {
			return nil, err
		}
		score := int(scoreFloat)
		scores = append(scores, Score{
			Name:  member,
			Score: score,
		})
	}
	return scores, nil
}

var errUnexpectedDataType = fmt.Errorf("unexpected data type in message")

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	var score Score
	if err := json.NewDecoder(r.Body).Decode(&score); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := client.ZAdd(r.Context(), "leaderboard", dicedb.Z{
		Score:  float64(score.Score),
		Member: score.Name,
	}).Err()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
