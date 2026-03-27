package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

//go:embed index.html
var content embed.FS

type Game struct {
	mu     sync.Mutex
	Board  [3][3]string `json:"board"`
	Turn   string       `json:"turn"`
	Winner string       `json:"winner"`
	Draw   bool         `json:"draw"`
}

func NewGame() *Game {
	return &Game{Turn: "X"}
}

func (g *Game) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Board = [3][3]string{}
	g.Turn = "X"
	g.Winner = ""
	g.Draw = false
}

func (g *Game) MakeMove(row, col int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Winner != "" || g.Draw {
		return fmt.Errorf("game is over")
	}
	if row < 0 || row > 2 || col < 0 || col > 2 {
		return fmt.Errorf("invalid position")
	}
	if g.Board[row][col] != "" {
		return fmt.Errorf("cell already occupied")
	}

	g.Board[row][col] = g.Turn
	g.checkWinner()

	if g.Winner == "" && !g.Draw {
		if g.Turn == "X" {
			g.Turn = "O"
		} else {
			g.Turn = "X"
		}
	}
	return nil
}

func (g *Game) checkWinner() {
	b := g.Board
	lines := [][3][2]int{
		{{0, 0}, {0, 1}, {0, 2}},
		{{1, 0}, {1, 1}, {1, 2}},
		{{2, 0}, {2, 1}, {2, 2}},
		{{0, 0}, {1, 0}, {2, 0}},
		{{0, 1}, {1, 1}, {2, 1}},
		{{0, 2}, {1, 2}, {2, 2}},
		{{0, 0}, {1, 1}, {2, 2}},
		{{0, 2}, {1, 1}, {2, 0}},
	}
	for _, line := range lines {
		a, bv, c := b[line[0][0]][line[0][1]], b[line[1][0]][line[1][1]], b[line[2][0]][line[2][1]]
		if a != "" && a == bv && bv == c {
			g.Winner = a
			return
		}
	}
	filled := 0
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			if b[r][c] != "" {
				filled++
			}
		}
	}
	if filled == 9 {
		g.Draw = true
	}
}

func (g *Game) State() map[string]interface{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return map[string]interface{}{
		"board":  g.Board,
		"turn":   g.Turn,
		"winner": g.Winner,
		"draw":   g.Draw,
	}
}

func main() {
	game := NewGame()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := content.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(game.State())
	})

	http.HandleFunc("/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Row int `json:"row"`
			Col int `json:"col"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := game.MakeMove(req.Row, req.Col); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(game.State())
	})

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		game.Reset()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(game.State())
	})

	log.Println("Tic-Tac-Toe server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

