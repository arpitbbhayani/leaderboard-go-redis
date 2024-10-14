package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var (
	client   *redis.Client
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	connectedUsers []*websocket.Conn
)

type Score struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

func main() {
	client = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	go watchLoop()

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

	connectedUsers = append(connectedUsers, conn)
}

func watchLoop() {
	for {
		scores, err := getTopScores()
		if err != nil {
			log.Println(err)
			return
		}

		for _, conn := range connectedUsers {
			if err := conn.WriteJSON(scores); err != nil {
				log.Println("websocket write error:", err)
				// TODO: remove the connection from the list
			}
		}

		time.Sleep(1 * time.Second)
	}
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	var score Score
	if err := json.NewDecoder(r.Body).Decode(&score); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := client.ZAdd(r.Context(), "leaderboard", &redis.Z{
		Score:  float64(score.Score),
		Member: score.Name,
	}).Err()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getTopScores() ([]Score, error) {
	cmd := client.ZRevRangeWithScores(client.Context(), "leaderboard", 0, 5)
	result, err := cmd.Result()
	if err != nil {
		return nil, err
	}

	var scores []Score
	for _, z := range result {
		scores = append(scores, Score{
			Name:  z.Member.(string),
			Score: int(z.Score),
		})
	}

	return scores, nil
}
