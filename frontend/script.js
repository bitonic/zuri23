
const canvas = document.getElementById("notebook");
canvas.style.cursor = 'none';
const ctx = canvas.getContext("2d");

ctx.fillStyle = "rgb(0, 100, 100)";
ctx.font = "30px monospace";
var clientId = 0;
var puzzleId = -1;

var tokens = [
    { txt: "...", X: 50 + Math.random() * 100, Y: 50 + Math.random() * 500 },
];

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

const wsUrl = new URL('/ws', window.location.href);
wsUrl.protocol = wsUrl.protocol.replace('http', 'ws');
wsUrl.href
const socket = new WebSocket(wsUrl.href);

socket.addEventListener("message", (event) => {
	console.log("message:", event.data);
        const obj = JSON.parse(event.data);
        for (let i in obj.Tokens) {
            if (i != clientId) {
                tokens[i] = obj.Tokens[i];
            }
        }

        if (obj.PuzzleID > puzzleId) {
	        // New puzzle!
	        puzzleId = obj.PuzzleID;
	        clientId = Math.floor(Math.random() * tokens.length);
                tokens = obj.Tokens;
	        document.getElementById("goal").innerHTML = obj.PuzzleGoal

	        console.log("clientId: ", clientId);
        }
        console.log(obj);
        document.getElementById("ghci").innerHTML = obj.GHCIOutput;
        render();
})

// A client update waiting to be sent to the server
var dataToSend = null;

function send() {
    const timerExists = dataToSend !== null;
    dataToSend = JSON.stringify({
        "ClientID": clientId,
        "PuzzleID": puzzleId,
        "X": tokens[clientId].X,
        "Y": tokens[clientId].Y}
    );
    if (!timerExists) {
        setTimeout(() => {
            if (dataToSend !== null) {
                socket.send(dataToSend);
                dataToSend = null;
            }
        }, 100);
    }
}
