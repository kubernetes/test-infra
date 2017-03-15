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
  self.logout = logout;
  self.isGuest = isGuest;
  self.getUser = getUser;
  self.doLogin = doLogin;

  dataService.getData("bulkprs/prs").then(function success(response) {
    self.prs = response.data;
  }, function error(response) {
    console.log("Error: Getting pr data: " + response);
  });

  getUser();

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

  function getUser() {
    dataService.getData("bulkprs/user").then(function success(response) {
      self.login = response.data.login;
    }, function error(response) {
      if (response.status == 404) {
        self.login = 'Guest'
      } else {
        console.log('error getting user: ' + response);
      }
    });
  }

  function isGuest() {
    return self.login == 'Guest';
  }

  function doLogin() {
    window.location = "bulkprs/auth?redirect=" + window.location;
  }

  function logout() {
    dataService.getData("bulkprs/auth?logout=true").then(
      function success() {
        self.getUser();
      }.bind(self),
      function error(response) { console.log(response); }
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
