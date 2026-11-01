const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

for (const page of ['index', 'register', 'chat']) {
    const html = fs.readFileSync(path.join(__dirname, page + '.html'), 'utf8');
    for (const match of html.matchAll(/<script>([\s\S]*?)<\/script>/g)) {
        new vm.Script(match[1], { filename: page + '.html' });
    }
    assert.ok(html.includes('src="status.js"'));
    assert.ok(html.includes('attachWebsocketStatus(bridge)'));
}

function setup() {
    const icon = { style: {}, setAttribute(name, value) { this[name] = value; } };
    const context = vm.createContext({ document: {
        createElement: () => icon, body: { appendChild() {} }
    } });
    vm.runInContext(fs.readFileSync(path.join(__dirname, 'status.js'), 'utf8'), context);
    let listener, snapshot;
    context.attachWebsocketStatus({
        server_event: { connect(callback) { listener = callback; } },
        get_websocket_status(callback) { snapshot = callback; }
    });
    return { icon, event(status) { listener(JSON.stringify({ type: 'websocket_status', status })); }, snapshot };
}

const first = setup();
assert.match(first.icon.title, /未連線/);
first.snapshot('green');
assert.match(first.icon.title, /已連線/);
first.event('yellow');
assert.match(first.icon.title, /重新連線/);
first.event('green');
assert.match(first.icon.title, /已連線/);
first.event('red');
assert.match(first.icon.title, /未連線/);
const race = setup();
race.event('green');
race.snapshot('red');
assert.match(race.icon.title, /已連線/);
console.log('Page script syntax and WebSocket status tests passed');