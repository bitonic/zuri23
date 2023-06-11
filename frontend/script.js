const canvas = document.getElementById("notebook");
canvas.style.cursor = 'none';
const ctx = canvas.getContext("2d");
const tokens = [
    { txt: "iterate", x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "take",    x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "10",      x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "$",       x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "(+1)",    x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
    { txt: "0",       x: 50 + Math.random() * 100, y: 50 + Math.random() * 500 },
];
ctx.fillStyle = "rgb(0, 100, 100)";
ctx.font = "30px monospace";
const clientId = Math.floor(Math.random() * tokens.length);
canvas.addEventListener("mousemove", e => {
    const rect = canvas.getBoundingClientRect();
    tokens[clientId].x = e.pageX - rect.left;
    tokens[clientId].y = e.pageY - rect.top;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    for (let i in tokens) {
        const token = tokens[i];
        ctx.fillText(token.txt, token.x, token.y);
    }
});
async function send() {
    try {
        const result = await fetch('/post', {
            method: 'POST',
            body: JSON.stringify(tokens[clientId]),
        });
        document.getElementById("ghci").innerHTML = await result.json();
    } finally {
        setTimeout(send, 100);
    }
}
send();
