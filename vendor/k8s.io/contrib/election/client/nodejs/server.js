const http = require('http');
let master = {};

const cb = (response) => {
  let data = '';
  response.on('data', (piece) => {
    data = data + piece;
  });
  response.on('end', () => {
    master = JSON.parse(data);
  });
};

const updateMaster = function updateMaster() {
  const req = http.get({
    host: 'localhost',
    path: '/',
    port: 4040,
  }, cb);

  req.on('error', (e) => {
    console.log(`problem with request: ${e.message}`);
  });
  req.end();
};

updateMaster();
setInterval(updateMaster, 5000);

const www = http.createServer((request, response) => {
  response.writeHead(200);
  response.end(`Master is ${master.name}`);
});

www.listen(8080);
