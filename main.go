package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var (
	redisClient *redis.Client
	upgrader    = websocket.Upgrader{
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
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
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

	for {
		scores, err := getTopScores()
		if err != nil {
			log.Println(err)
			return
		}

		if err := conn.WriteJSON(scores); err != nil {
			log.Println(err)
			return
		}

		redisClient.Wait(context.TODO(), 1, 0)
	}
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	var score Score
	if err := json.NewDecoder(r.Body).Decode(&score); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := redisClient.ZAdd(r.Context(), "leaderboard", &redis.Z{
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
	cmd := redisClient.ZRevRangeWithScores(redisClient.Context(), "leaderboard", 0, 5)
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
