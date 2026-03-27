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













































































































































































































































</html>`</body></script>  fetchState();  }    render(await res.json());    const res = await fetch("/reset", {method: "POST"});  async function resetGame() {  }    render(await res.json());    });      body: JSON.stringify({row, col})      headers: {"Content-Type": "application/json"},      method: "POST",    const res = await fetch("/move", {  async function makeMove(row, col) {  }    render(await res.json());    const res = await fetch("/state");  async function fetchState() {  }    }      status.textContent = "Player " + state.turn + "'s turn";    } else {      status.textContent = "It's a draw!";    } else if (state.draw) {      status.textContent = "Player " + state.winner + " wins!";    if (state.winner) {    }      }        board.appendChild(cell);        if (!v && !over) cell.addEventListener("click", () => makeMove(r, c));        if (v || over) cell.classList.add("disabled");        if (v === "O") cell.classList.add("o");        if (v === "X") cell.classList.add("x");        cell.textContent = v;        const v = state.board[r][c];        cell.className = "cell";        const cell = document.createElement("button");      for (let c = 0; c < 3; c++) {    for (let r = 0; r < 3; r++) {    const over = state.winner || state.draw;    board.innerHTML = "";  function render(state) {  const status = document.getElementById("status");  const board = document.getElementById("board");<script></div>  <button id="resetBtn" onclick="resetGame()">New Game</button>  <div class="board" id="board"></div>  <div id="status">Loading...</div>  <h1>Tic-Tac-Toe</h1><div class="container"><body></head></style>  #resetBtn:hover { background: #c73652; }  #resetBtn { margin-top: 1.2em; padding: 0.6em 2em; font-size: 1em; background: #e94560; color: #fff; border: none; border-radius: 6px; cursor: pointer; }  .cell.disabled:hover { background: #0f3460; }  .cell.disabled { cursor: default; }  .cell.o { color: #0cc0df; }  .cell.x { color: #e94560; }  .cell:hover { background: #1a4a7a; }  .cell { width: 100px; height: 100px; background: #0f3460; border: none; border-radius: 6px; font-size: 2.5em; font-weight: bold; color: #eee; cursor: pointer; transition: background 0.15s; display: flex; align-items: center; justify-content: center; }  .board { display: inline-grid; grid-template-columns: repeat(3, 100px); grid-template-rows: repeat(3, 100px); gap: 6px; background: #16213e; padding: 6px; border-radius: 10px; }  #status { font-size: 1.3em; margin-bottom: 1em; min-height: 1.6em; }  h1 { margin-bottom: 0.5em; font-size: 2em; }  .container { text-align: center; }  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; background: #1a1a2e; color: #eee; }  * { margin: 0; padding: 0; box-sizing: border-box; }<style><title>Tic-Tac-Toe</title><meta name="viewport" content="width=device-width, initial-scale=1.0"><meta charset="UTF-8"><head><html lang="en">const indexHTML = `<!DOCTYPE html>}	log.Fatal(http.ListenAndServe(":8080", nil))	log.Println("Tic-Tac-Toe server listening on :8080")	})		json.NewEncoder(w).Encode(game.State())		w.Header().Set("Content-Type", "application/json")		game.Reset()		}			return			http.Error(w, "POST only", http.StatusMethodNotAllowed)		if r.Method != http.MethodPost {	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {	})		json.NewEncoder(w).Encode(game.State())		w.Header().Set("Content-Type", "application/json")		}			return			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})			w.WriteHeader(http.StatusBadRequest)			w.Header().Set("Content-Type", "application/json")		if err := game.MakeMove(req.Row, req.Col); err != nil {		}			return			http.Error(w, "bad request", http.StatusBadRequest)		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {		}			Col int `json:"col"`			Row int `json:"row"`		var req struct {		}			return			http.Error(w, "POST only", http.StatusMethodNotAllowed)		if r.Method != http.MethodPost {	http.HandleFunc("/move", func(w http.ResponseWriter, r *http.Request) {	})		json.NewEncoder(w).Encode(game.State())		w.Header().Set("Content-Type", "application/json")	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {	})		fmt.Fprint(w, indexHTML)		w.Header().Set("Content-Type", "text/html; charset=utf-8")		}			return			http.NotFound(w, r)		if r.URL.Path != "/" {	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {	game := NewGame()func main() {}	}		"draw":   g.Draw,		"winner": g.Winner,		"turn":   g.Turn,		"board":  g.Board,	return map[string]interface{}{	defer g.mu.Unlock()	g.mu.Lock()func (g *Game) State() map[string]interface{} {}	}		g.Draw = true	if filled == 9 {	}		}			}				filled++			if b[r][c] != "" {		for c := 0; c < 3; c++ {	for r := 0; r < 3; r++ {	filled := 0	}		}			return			g.Winner = a		if a != "" && a == bv && bv == c {		a, bv, c := b[line[0][0]][line[0][1]], b[line[1][0]][line[1][1]], b[line[2][0]][line[2][1]]	for _, line := range lines {	}		{{0, 2}, {1, 1}, {2, 0}},		{{0, 0}, {1, 1}, {2, 2}},		{{0, 2}, {1, 2}, {2, 2}},		{{0, 1}, {1, 1}, {2, 1}},		{{0, 0}, {1, 0}, {2, 0}},		{{2, 0}, {2, 1}, {2, 2}},		{{1, 0}, {1, 1}, {1, 2}},		{{0, 0}, {0, 1}, {0, 2}},	lines := [][3][2]int{	b := g.Boardfunc (g *Game) checkWinner() {}	return nil	}		}			g.Turn = "X"		} else {			g.Turn = "O"		if g.Turn == "X" {	if g.Winner == "" && !g.Draw {	g.checkWinner()	g.Board[row][col] = g.Turn	}		return fmt.Errorf("cell already occupied")	if g.Board[row][col] != "" {	}		return fmt.Errorf("invalid position")	if row < 0 || row > 2 || col < 0 || col > 2 {	}		return fmt.Errorf("game is over")	if g.Winner != "" || g.Draw {	defer g.mu.Unlock()	g.mu.Lock()func (g *Game) MakeMove(row, col int) error {}	g.Draw = false	g.Winner = ""	g.Turn = "X"	g.Board = [3][3]string{}	defer g.mu.Unlock()	g.mu.Lock()func (g *Game) Reset() {}	return &Game{Turn: "X"}func NewGame() *Game {}	Draw   bool         `json:"draw"`	Winner string       `json:"winner"`	Turn   string       `json:"turn"`	Board  [3][3]string `json:"board"`	mu     sync.Mutextype Game struct {)	"sync"	"net/http"	"log"	"fmt"	"encoding/json"import (package main