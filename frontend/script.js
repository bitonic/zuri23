const canvas = document.getElementById("notebook");
canvas.style.cursor = 'none';
const ctx = canvas.getContext("2d");

var tokens = [
    { txt: "...", X: 50 + Math.random() * 100, Y: 50 + Math.random() * 500 },
]

ctx.fillStyle = "rgb(0, 100, 100)";
ctx.font = "30px monospace";
var clientId = 0;
var puzzleId = -1;

canvas.addEventListener("mousemove", e => {
    const rect = canvas.getBoundingClientRect();
    tokens[clientId].X = e.pageX - rect.left;
    tokens[clientId].Y = e.pageY - rect.top;
    render();
    send();
});

function render() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    for (let i in tokens) {
        const token = tokens[i];
        ctx.fillText(token.Token, token.X, token.Y);
    }
}

const socket = new WebSocket("ws://localhost:8001/ws");

socket.addEventListener("message", (event) => {
	console.log("message:", event.data);
        const obj = event.data;
        tokens = obj.Tokens;

        if (obj.PuzzleID > puzzleId) {
	        // New puzzle!
	        puzzleId = obj.PuzzleID;
	        clientId = Math.floor(Math.random() * tokens.length);
	        document.getElementById("goal").innerHTML = obj.PuzzleGoal

	        console.log("clientId: ", clientId);
        }
        console.log(obj);
        document.getElementById("ghci").innerHTML = obj.GHCIOutput;
        render();
})

async function send() {
	socket.send(
		JSON.stringify({
	            "ClientID": clientId,
	            "PuzzleID": puzzleId,
	            "X": tokens[clientId].X,
	            "Y": tokens[clientId].Y}),
        );
}
