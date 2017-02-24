"use strict";
var app = angular.module('DiffModule', ['ngMaterial']);

app.config(['$compileProvider', function ($compileProvider) {
  $compileProvider.debugInfoEnabled(false);
}]);

app.controller('DiffCntl', ['DataService', '$location', DiffCntl]);

function DiffCntl(dataService, $location) {
  var self = this;
  var subsearch = window.location.search.substr(1);
  var obj = {};
  var parts = subsearch.split('&');
  for (var i = 0; i < parts.length; i++) {
      var pieces = parts[i].split('=');
      obj[pieces[0]] = pieces[1];
  }
  self.query = obj;

  if (self.query['pr']) {
      console.log('FOO');
      dataService.getData('bulkprs/prdiff?number=' + self.query['pr']).then(
        function success(response) {
            console.log(response);

            var diff2htmlUi = new Diff2HtmlUI({diff: response.data});
            diff2htmlUi.draw('#diff', {inputFormat: 'json', showFiles: true, matching: 'lines'});
        },
        function error(response) {
            console.log(response);
        }
      );
  }
}

app.service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData(file) {
    return $http.get(file);
  }
}
