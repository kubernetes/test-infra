"use strict";
angular.module('SubmitQueueModule', ['ngMaterial', 'md.data.table']);

angular.module('SubmitQueueModule').controller('SQCntl', ['DataService', '$interval', SQCntl]);

function SQCntl(dataService, $interval) {
  var self = this;
  self.prDisplayValue = "";
  self.prs = {};
  self.users = {};
  self.builds = {};
  self.querySearch = querySearch;
  self.updatePRVisibility = updatePRVisibility;
  self.queryNum = 0;
  self.StatOrder = "-Count";

  // Refresh data every 60 seconds
  refresh();
  $interval(refresh, 60000);

  function refresh() {
    dataService.getData('api').then(function successCallback(response) {
      var prs = getPRs(response.data.PRStatus);
      __updatePRVisibility(prs);
      self.prs = prs;
      self.prSearchTerms = getPRSearchTerms();
    }, function errorCallback(response) {
      console.log("Error: Getting SubmitQueue Status");
    });
  }

  // Refresh every 30 seconds
  refreshGithubE2E();
  $interval(refreshGithubE2E, 30000);

  function refreshGithubE2E() {
    dataService.getData('github-e2e-queue').then(function successCallback(response) {
      if (response.data.E2ERunning.Number === "") {
        self.e2erunning = [];
      } else {
        self.e2erunning = [response.data.E2ERunning];
      }
      self.e2equeue = response.data.E2EQueue;
    });
  }

  // Refresh every 30 seconds
  refreshMessages();
  $interval(refreshMessages, 30000);

  function refreshMessages() {
    dataService.getData('messages').then(function successCallback(response) {
      var msgs = getPRs(response.data);
      __updatePRVisibility(msgs);
      self.statusMessages = msgs;
    });
  }

  // Refresh every 15 minutes
  refreshStats();
  $interval(refreshStats, 900000);

  function refreshStats() {
    dataService.getData('stats').then(function successCallback(response) {
      var d = new Date(response.data.NextLoopTime);
      document.getElementById("next-run-time").innerHTML = d.toString();

      self.botStats = getStats(response.data.Analytics);
      document.getElementById("api-calls").innerHTML = response.data.APICount;
      document.getElementById("api-calls-per-sec").innerHTML = response.data.APIPerSec;
    });
  }

  // Refresh every 15 minutes
  refreshUsers();
  $interval(refreshUsers, 900000);

  function refreshUsers() {
    dataService.getData('users').then(function successCallback(response) {
      self.users = response.data;
    });
  }

  // Refresh every minute
  refreshGoogleInternalCI();
  $interval(refreshGoogleInternalCI, 60000);

  function refreshGoogleInternalCI() {
    dataService.getData('google-internal-ci').then(function successCallback(response) {
      var result = getE2E(response.data);
      self.builds = result.builds;
      self.failedBuild = result.failedBuild;
    });
  }

  function __updatePRVisibility(prs) {
    angular.forEach(prs, function(pr) {
      if (typeof self.prDisplayValue === "undefined") {
        pr.show = true;
      } else if (pr.Login.toLowerCase().match("^" + self.prDisplayValue.toLowerCase()) || pr.Number.match("^" + self.prDisplayValue)) {
        pr.show = true;
      } else {
        pr.show = false;
      }
    });
  }

  function updatePRVisibility() {
    __updatePRVisibility(self.prs);
  }

  function getPRs(prs) {
    var result = [];
    angular.forEach(prs, function(value, key) {
      var obj = {
        'Num': key
      };
      angular.forEach(value, function(value, key) {
        if (key === "Time") {
          var d = new Date(value);
          value = d.toString();
        }
        obj[key] = value;
      });
      result.push(obj);
    });
    return result;
  }

  function getStats(stats) {
    var result = [];
    angular.forEach(stats, function(value, key) {
      var obj = {
        Name: key,
        Count: value
      };
      result.push(obj);
    });
    return result
  }

  function getE2E(builds) {
    var result = [];
    var failedBuild = false;
    angular.forEach(builds, function(value, key) {
      var obj = {
        'name': key
      };
      if (value == 'Stable') {
        // green check mark
        obj.state = '\u2713';
        obj.color = 'green';
      } else if (value == 'Not Stable') {
        // red X mark
        obj.state = '\u2716';
        obj.color = 'red';
        failedBuild = true;
      } else {
        obj.state = 'Error';
        obj.color = 'red';
        obj.msg = value;
        failedBuild = true;
      }
      result.push(obj);
    });
    return {
      builds: result,
      failedBuild: failedBuild,
    };
  }

  function searchTermsContain(terms, value) {
    var found = false;
    angular.forEach(terms, function(term) {
      if (term.value === value) {
        found = true;
      }
    });
    return found;
  }

  function getPRSearchTerms() {
    var result = [];
    angular.forEach(self.prs, function(pr) {
      var llogin = pr.Login.toLowerCase();
      if (!searchTermsContain(result, llogin)) {
        var loginobj = {
          value: llogin,
          display: pr.Login,
        };
        result.push(loginobj);
      }
      if (!searchTermsContain(result, pr.Num)) {
        var numobj = {
          value: pr.Num,
          display: pr.Num,
        };
        result.push(numobj);
      }
    });
    result.sort(compareSearchTerms);
    return result;
  }

  /* We need to compare the 'value' to get a sane sort */
  function compareSearchTerms(a, b) {
    if (a.value > b.value) {
      return 1;
    }
    if (a.value < b.value) {
      return -1;
    }
    return 0;
  }

  function querySearch(query) {
    var results = query ? self.prSearchTerms.filter(createFilterFor(query)) : self.prSearchTerms;
    return results;
  }

  /**
   * Create filter function for a query string
   */
  function createFilterFor(query) {
    var lowercaseQuery = angular.lowercase(query);
    return function filterFn(state) {
      return (state.value.indexOf(lowercaseQuery) === 0);
    };
  }

}

function goToPerson(person) {
  window.location.href = 'https://github.com/' + person;
}

angular.module('SubmitQueueModule').service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData(file) {
    return $http.get(file);
  }
}
