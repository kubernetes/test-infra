'use strict';

const PARAMS = 5;
const HOST = 'github.com';

var logger = {
  data: [],
  message: '',
  options: {
    request: {
      endpoint: 'https://' + window.location.hostname + '/trace?',
      params: '',
      config: {}
    },
    messages: {
      invalidInput: 'Invalid input. Please pass a link from a PR or PR comment and profit',
      invalidHost: '{value} is an invalid host. Host needs to be github.com',
      invalidParams: 'The following {value} provided as params. Need either "pr", "repo", and "org", or  "issuecomment" to be specified',
      invalidUrl: '{value} is an invalid link. Failed to construct \'URL\'',
      requestError: 'Fetch error: Status is: {value}',
      requestLoading: 'Loading..',
      emptyLogs: 'No logs to display'
    }
  },

  // the actual fetch
  request: function(endpoint) {
    generateView.toggleElement(generateView.loader, true);
    fetch(endpoint, {credentials: 'same-origin'}).then(
        function(response) {

          if (response.status !== 200) {
            return Promise.reject(response.status);
          } else {
            generateView.toggleElement(generateView.messageHolder, false);
          }

          response.json().then(function(data) {
            controller.setData(data);
          });
        }
    ).catch(function(err) {
      controller.setMessage(logger.options.messages.requestError, err);
    }).finally(function(){
      generateView.toggleElement(generateView.loader, false);
    });

  }
};

var controller = {

  init: function() {
    generateView.init();
  },

  // Getter & Setters

  setParams: function(params) {
    logger.options.request.params = params;
  },

  setData: function(data) {
    logger.data = data;
    // render this view (update the DOM elements with the updated values)
    generateView.render();
  },

  getParams: function() {
    return logger.options.request.endpoint + encodeURI(logger.options.request.params);
  },

  setMessage: function(msg, value) {
    generateView.toggleElement(generateView.messageHolder, true);
    logger.message = msg.replace('{value}', value);
    generateView.updateMsg();
  },

  getMessage: function() {
    return logger.message;
  },

  // fetch data
  requestLogs: function() {
    //  params needed
    return logger.request(this.getParams());
  },

  buildParams: function(userValue) {

    if (userValue.length <= 1) {
      this.setMessage(logger.options.messages.invalidInput);
      return;
    }

    try {
      var url = new URL(userValue);
    } catch(err) {
      this.setMessage(logger.options.messages.invalidUrl, userValue);
      return;
    }

    var host = url.host,
        urlParser = url.pathname.split('/');

    if (host !== HOST) {
      this.setMessage(logger.options.messages.invalidHost, userValue);
      return;
    }

    /* 
      A valid PR link has 4 params. The params are: org, repo, pr number and pull 
      Although the length of urlParser is 5 because of split('/'). 
      It splits the url into an array of params with the separator "/" so it returns 5.
    */
    if (urlParser.length !== PARAMS) {
      this.setMessage(logger.options.messages.invalidParams, urlParser);
      return;
    }

    var org = urlParser[1],
        repo = urlParser[2],
        pr = urlParser[4];

    var params = 'org=' + org + '&repo=' + repo + '&pr=' + pr;

    if (url.hash.length > 1) {
      params += '&issuecomment=' + url.hash.substr(1).replace('-', '=');
    }

    this.setParams(params);
    this.requestLogs();

  }

};

var generateView = {
  init: function() {
    // store pointers to our DOM elements for easy access later

    this._columnHeaders_ = ["time", "level", "msg", "from", "to", "job", "event-type", "", "type"];
    this._table_ = document.createElement('table');
    this._tr_ = document.createElement('tr');
    this._th_ = document.createElement('th');
    this._td_ = document.createElement('td');
    this._ul_ = document.createElement('ul');
    this._li_ = document.createElement('li');
    this.response = document.getElementById('response');
    this.messageHolder = document.getElementById('error-message');
    this.userInput = document.getElementById('user-input');
    this.searchSubmit = document.getElementById('search-submit');
    this.loader = document.getElementById('loading');
    this.searchWrapper = document.getElementById('search-wrapper');


    // on click, get the user input and build the param
    this.searchSubmit.addEventListener('click', function() {
      var userValue = generateView.userInput.value;

      controller.buildParams(userValue);
    });

    // event listener for Enter
    this.userInput.addEventListener("keyup", function(event) {
      event.preventDefault();
      if (event.keyCode === 13) {
        generateView.searchSubmit.click();
      }
    });
  },

  render: function() {
    var logs = logger.data;
    //clear response each time
    this.response.innerHTML = "";
    // check if there are logs to display
    if ( logs.length > 0) {
      this.searchWrapper.classList.add("top");
      this.response.appendChild(this.buildHtmlTable(logs));
    } else {
      this.response.innerText = logger.options.messages.emptyLogs;
    }
  },

  // Builds the HTML Table out of json data.

  buildHtmlTable: function(arr) {

    var table = this._table_.cloneNode(false),
        columns = this.addColumnHeaders(arr, table),
        extra = this.addExtra(arr, table);

    for (var i = 0, maxi = arr.length; i < maxi; ++i) {

      var tr = this._tr_.cloneNode(false),
          ul = this._ul_.cloneNode(false);

      // append the basic columns
      for (var j = 0, maxj = columns.length; j < maxj; ++j) {
        var td = this._td_.cloneNode(false);

        var tableData  = arr[i][columns[j]] || '';

        if (columns[j] === "time") {
          tableData  = new Date(arr[i][columns[j]]) ;
        }

        td.appendChild(document.createTextNode(tableData));
        tr.appendChild(td);
      }


      // append the rest log columns
      for (var c = 0, maxc = extra.length; c < maxc; ++c) {
        var li = this._li_.cloneNode(false);

        if (extra[c] && arr[i][extra[c]]) {
          var extraLogInfo = extra[c] + ": " + arr[i][extra[c]];
          li.appendChild(document.createTextNode(extraLogInfo));
          ul.appendChild(li);
          tr.appendChild(ul);
        }

      }

      table.appendChild(tr);
    }

    return table;
  },

  addColumnHeaders: function(arr, table) {
    // set default column headers for the table
    var columnSet =  this._columnHeaders_,
        tr = this._tr_.cloneNode(false);

    //build table header
    for (var counter= 0, columnLength = columnSet.length; counter < columnLength; counter++ ) {

      var th = this._th_.cloneNode(false);
      th.appendChild(document.createTextNode(columnSet[counter]));
      tr.appendChild(th);

    }

    table.appendChild(tr);
    return columnSet;

  },

  addExtra: function(arr) {

    var extraSet = [],
        columnSet =  this._columnHeaders_;

    for (var i = 0, l = arr.length; i < l; i++) {
      for (var key in arr[i]) {
        if (arr[i].hasOwnProperty(key) && columnSet.indexOf(key) === -1 && extraSet.indexOf(key) === -1) {
          extraSet.push(key);
        }
      }
    }

    return extraSet;

  },

  updateMsg: function() {
    this.messageHolder.innerText = controller.getMessage();
  },

  toggleElement: function(elem, show) {
    show? elem.classList.remove("hide") : elem.classList.add("hide");
  }
};

document.addEventListener('DOMContentLoaded', function() {
  controller.init();
});

