const canvas = document.getElementById("notebook");
canvas.style.cursor = 'none';
const ctx = canvas.getContext("2d");
/*const tokens = [
    { txt: "iterate", x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "take",    x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "10",      x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "$",       x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "(+1)",    x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "0",       x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
];*/

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
});

function render() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    for (let i in tokens) {
        const token = tokens[i];
        ctx.fillText(token.Token, token.X, token.Y);
    }
}

async function send() {
    try {
        const result = await fetch('/post', {
            method: 'POST',
            body: JSON.stringify({
	            "ClientID": clientId,
	            "PuzzleID": puzzleId,
	            "X": tokens[clientId].X,
	            "Y": tokens[clientId].Y}),
        });
        const obj = await result.json();
        tokens = obj.Tokens;

        if (obj.PuzzleID > puzzleId) {
	        // New puzzle!
	        puzzleId = obj.PuzzleID;
	        clientId = Math.floor(Math.random() * tokens.length);

	        console.log("clientId: ", clientId);
        }
        console.log(obj);
        document.getElementById("ghci").innerHTML = obj.GHCIOutput;
        render();
    } finally {
        setTimeout(send, 100);
    }
}
send();
