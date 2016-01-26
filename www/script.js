"use strict";
angular.module('SubmitQueueModule', ['ngMaterial', 'md.data.table', 'angular-toArrayFilter']);

angular.module('SubmitQueueModule').controller('SQCntl', ['DataService', '$interval', '$location', SQCntl]);

function SQCntl(dataService, $interval, $location) {
  var self = this;
  self.prs = {};
  self.users = {};
  self.builds = {};
  self.prQuerySearch = prQuerySearch;
  self.historyQuerySearch = historyQuerySearch;
  self.goToPerson = goToPerson;
  self.queryNum = 0;
  self.StatOrder = "-Count";

  // http://submit-queue.k8s.io/#?prDisplay=eparis&historyDisplay=15999
  //  will show all PRs opened by eparis and all historic decisions for PR #15999
  var vars = $location.search()
  self.prDisplayValue = vars.prDisplay
  self.historyDisplayValue = vars.historyDisplay

  // Refresh data every 10 minutes
  refreshPRs();
  $interval(refreshPRs, 600000);

  function refreshPRs() {
    dataService.getData('prs').then(function successCallback(response) {
      var prs = response.data.PRStatus;
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
      if (response.data.E2ERunning.Number == 0) {
        self.e2erunning = [];
      } else {
        self.e2erunning = [response.data.E2ERunning];
      }
      self.e2equeue = response.data.E2EQueue;
      document.getElementById("queue-len").innerHTML = "&nbsp;(" + self.e2equeue.length + ")"
    });
  }

  // Refresh every minute
  refreshHistoryPRs();
  $interval(refreshHistoryPRs, 60000);

  function refreshHistoryPRs() {
    dataService.getData('history').then(function successCallback(response) {
      var prs = response.data;
      self.historyPRs = prs;
      self.historySearchTerms = getHistorySearchTerms();
    });
  }

  // Refresh every 10 minutes
  refreshStats();
  $interval(refreshStats, 600000);

  function refreshStats() {
    dataService.getData('stats').then(function successCallback(response) {
      var nextLoop = new Date(response.data.NextLoopTime);
      document.getElementById("next-run-time").innerHTML = nextLoop.toLocaleTimeString();

      self.botStats = response.data.Analytics;
      self.APICount = response.data.APICount;
      self.CachedAPICount = response.data.CachedAPICount;
      document.getElementById("api-calls-per-sec").innerHTML = response.data.APIPerSec;
      document.getElementById("github-api-limit-count").innerHTML = response.data.LimitRemaining;
      var nextReset = new Date(response.data.LimitResetTime);
      document.getElementById("github-api-limit-reset").innerHTML = nextReset.toLocaleTimeString();
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

  function getE2E(builds) {
    var result = [];
    var failedBuild = false;
    angular.forEach(builds, function(job, key) {
      var obj = {
        'name': key,
        'id': job.ID,
      };
      if (job.Status == 'Stable') {
        // green check mark
        obj.state = '\u2713';
        obj.color = 'green';
      } else if (job.Status == 'Not Stable') {
        // red X mark
        obj.state = '\u2716';
        obj.color = 'red';
        failedBuild = true;
      } else {
        obj.state = 'Error';
        obj.color = 'red';
        obj.msg = job.Status;
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
    return getSearchTerms(self.prs);
  }

  function getHistorySearchTerms() {
    return getSearchTerms(self.historyPRs);
  }

  function getSearchTerms(prs) {
    var result = [];
    angular.forEach(prs, function(pr) {
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
          value: pr.Number.toString(),
          display: pr.Number,
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

  function prQuerySearch(query) {
    return querySearch(query, self.prSearchTerms);
  }

  function historyQuerySearch(query) {
    return querySearch(query, self.historySearchTerms);
  }

  function querySearch(query, terms) {
    var results = query ? terms.filter(createFilterFor(query)) : terms;
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

  function goToPerson(person) {
    console.log(person);
    window.location.href = 'https://github.com/' + person;
  }
}


angular.module('SubmitQueueModule').filter('loginOrPR', function() {
  return function(prs, searchVal) {
    searchVal = searchVal || "";
    prs = prs || [];
    searchVal = angular.lowercase(searchVal);
    var out = [];

    angular.forEach(prs, function(pr) {
      var shouldPush = false;
      var llogin = pr.Login.toLowerCase();
      if (llogin.indexOf(searchVal) === 0) {
        shouldPush = true;
      }
      if (pr.Number.toString().indexOf(searchVal) === 0) {
        shouldPush = true;
      }
      if (shouldPush) {
        out.push(pr);
      }
    });
    return out;
  };
});

angular.module('SubmitQueueModule').service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData(file) {
    return $http.get(file);
  }
}
