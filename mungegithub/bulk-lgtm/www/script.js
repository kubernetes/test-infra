"use strict";
var app = angular.module('BulkLGTMModule', ['ngMaterial']);

app.config(['$compileProvider', function ($compileProvider) {
  $compileProvider.debugInfoEnabled(false);
}]);

app.controller('BulkLGTMCntl', ['DataService', '$interval', '$location', BulkLGTMCntl]);

function BulkLGTMCntl(dataService, $interval, $location) {
  var self = this;
  self.prs = {};
  self.prsCount = 0;
  self.metadata = {};
  self.location = $location;
  self.lgtm = lgtm;

  dataService.getData("bulkprs/prs").then(function success(response) {
    self.prs = response.data;
  }, function error(response) {
    console.log("Error: Getting pr data: " + response);
  });

  function lgtm(number) {
    dataService.getData("bulkprs/lgtm?number=" + number).then(function success(response) {
      for (var i = 0; i < self.prs.length; i++) {
        if (self.prs[i].number == number) {
          self.prs.splice(i, 1);
          return;
        }
      }
    }, function error(response) {
      console.log("Error LGTM-ing PR: " + response);
    });
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
