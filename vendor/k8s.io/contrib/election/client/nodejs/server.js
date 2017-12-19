var http = require('http');
var master = {};

var handleRequest = function(request, response) {
    response.writeHead(200);
    response.end("Master is " + master.name);
};

var cb = function(response) {
    var data = '';
    response.on('data', function(piece) { data = data + piece; });
    response.on('end', function() { master = JSON.parse(data); });
};

var updateMaster = function() {
    var req = http.get({host: 'localhost', path: '/', port: 4040}, cb);
    req.on('error', function(e) { console.log('problem with request: ' + e.message); });
    req.end();
};

updateMaster();
setInterval(updateMaster, 5000);

var www = http.createServer(handleRequest);
www.listen(8080);
